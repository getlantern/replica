use crate::search::Index;

use rusoto_core::Region;
use rusoto_s3::*;
use rusoto_sns::*;
use rusoto_sqs::*;
use std::collections::HashMap;

use crate::handle;
use crate::Result;
use anyhow::*;

use futures::executor::block_on;
use log::*;
use serde_json::json;
use std::sync::Mutex;
use uuid::Uuid;

const REGION: Region = Region::ApSoutheast1;
const TEST_BOUNDARIES: bool = false;
const ACCOUNT_ID: &str = "670960738222";

pub async fn get_all_objects() -> Vec<Object> {
    let s3 = S3Client::new(REGION);
    let mut all: Vec<Object> = vec![];
    let mut token = None;
    loop {
        let req = ListObjectsV2Request {
            bucket: "getlantern-replica".to_string(),
            max_keys: if TEST_BOUNDARIES {
                Some(2)
            } else {
                Default::default()
            },
            continuation_token: token.clone(),
            ..Default::default()
        };
        let mut list = match s3.list_objects_v2(req).await {
            Ok(ok) => ok,
            Err(err) => {
                // TODO: Why don't we get an error if we have bad credentials?
                error!("error listing objects: {}", err);
                continue;
            }
        };
        if let Some(v) = list.contents.as_mut() {
            all.extend(v.drain(..));
        }
        let next = list.next_continuation_token;
        match next {
            Some(_) => token = next,
            None => break,
        }
    }
    all
}

// Note that the returned tokens do not include the UUID prefix.
pub fn tokenize_object_key(key: &str) -> Result<Vec<String>> {
    ensure!(key.len() >= 37, "key too short to contain uuid prefix");
    Uuid::parse_str(&key[..36]).with_context(|| format!("parsing uuid: {}", key))?;
    let name = &key[37..];
    let ok = Ok(crate::search::split_name(name)
        .map(ToString::to_string)
        .collect());
    debug!("tokenized {} to {:?}", key, ok.as_ref().unwrap());
    ok
}

fn handle_event(event: &Event, index: &Mutex<Index>) -> Result<()> {
    let mut index = index.lock().unwrap();
    match event.r#type {
        EventType::Added => index.add_key(
            &event.key,
            crate::search::KeyInfo {
                size: event.size.unwrap(),
            },
        ),
        EventType::Removed => index.remove_key(&event.key),
    }
}

pub async fn receive_s3_events(index: &Mutex<Index>, queue_url: &str) {
    let sqs = rusoto_sqs::SqsClient::new(REGION);
    loop {
        let input = rusoto_sqs::ReceiveMessageRequest {
            queue_url: queue_url.to_string(),
            // We use long-polling here, but wait for it to return before checking the stop flag.
            // Using None results in too many calls if the latency is low. TODO: Use the futures,
            // and do cancellation synchronously. Note that the maximum is Some(20).
            wait_time_seconds: Some(20),
            max_number_of_messages: Some(10),
            // visibility_timeout: Some(0),
            ..Default::default()
        };
        let result = sqs.receive_message(input).await;
        trace!("receive_message returned");
        let result = handle!(result, err, {
            error!("error receiving messages: {}", err);
            continue;
        });
        trace!("result messages: {:#?}", result.messages);
        for msg in result.messages.unwrap_or_default() {
            let body = msg.body.unwrap();
            let _delete = sqs
                .delete_message(rusoto_sqs::DeleteMessageRequest {
                    queue_url: queue_url.to_owned(),
                    receipt_handle: msg.receipt_handle.unwrap(),
                })
                .await;
            debug!("got message: {:#?}", body);
            let records = handle!(get_records(body), err, {
                warn!("error getting records: {}", err);
                continue;
            });
            for r in records {
                let event = handle!(parse_record(r), err, {
                    error!("parsing record: {}", err);
                    continue;
                });
                handle!(handle_event(&event, index), err, {
                    error!("error handling event {:?}: {}", event, err);
                    continue;
                });
                info!("handled {:?}", event);
            }
        }
    }
}

#[derive(Debug)]
enum EventType {
    Added,
    Removed,
}

#[derive(Debug)]
struct Event {
    r#type: EventType,
    key: String,
    size: Option<crate::bittorrent::FileSize>,
}

use serde_json::Value as JsonValue;
use std::str::FromStr;

fn parse_record(rec: JsonValue) -> Result<Event> {
    let object = &rec["s3"]["object"];
    Ok(Event {
        r#type: {
            let event_name = rec["eventName"].as_str().unwrap();
            match event_name {
                "ObjectCreated:Put" | "ObjectCreated:CompleteMultipartUpload" => EventType::Added,
                "ObjectRemoved:Delete" => EventType::Removed,
                _ => bail!("unhandled event name {:?}", event_name),
            }
        },
        key: object["key"].as_str().unwrap().to_string(),
        size: rec["size"].as_i64(),
    })
}

fn get_records(body: String) -> Result<Vec<JsonValue>> {
    let value = JsonValue::from_str(body.as_str())?;
    let mut value = JsonValue::from_str(value["Message"].as_str().unwrap())?;
    if let JsonValue::Array(records) = value["Records"].take() {
        Ok(records)
    } else {
        bail!("Records array not found")
    }
}

fn queue_policy(queue_arn: &str) -> String {
    json!({
      "Version": "2012-10-17",
      "Id": format!("{}/SQSDefaultPolicy", queue_arn),
      "Statement": [
        {
          "Sid": "SNSSend",
          "Effect": "Allow",
          "Principal": {
            "AWS": "*"
          },
          "Action": "SQS:SendMessage",
          "Resource": queue_arn,
          "Condition": {
            "ArnEquals": {
              "aws:SourceArn": "arn:aws:sns:ap-southeast-1:670960738222:replica-search-events"
            }
          }
        },
        {
          "Sid": "SearcherRead",
          "Effect": "Allow",
          "Principal": {
            "AWS": "*"
          },
          "Action": "SQS:ReceiveMessage",
          "Resource": queue_arn,
        }
      ]
    })
    .to_string()
}

const CREATE_WITH_POLICY: bool = true;

pub struct EventQueue {
    pub url: String,
}

impl Drop for EventQueue {
    fn drop(&mut self) {
        block_on(delete_queue(&self.url))
    }
}

pub async fn create_event_queue(name: &str) -> EventQueue {
    let sqs = rusoto_sqs::SqsClient::new(REGION);
    let input = CreateQueueRequest {
        queue_name: name.to_string(),
        attributes: if CREATE_WITH_POLICY {
            Some({
                let mut attrs = HashMap::new();
                attrs.insert(
                    "Policy".to_string(),
                    queue_policy(&format!(
                        "arn:aws:sqs:{}:{}:{}",
                        REGION.name(),
                        ACCOUNT_ID,
                        name
                    )),
                );
                attrs
            })
        } else {
            None
        },
        ..Default::default()
    };
    let result = sqs.create_queue(input).await.unwrap();
    let queue = EventQueue {
        url: result.queue_url.unwrap(),
    };
    info!("created sqs queue {}", queue.url);
    if !CREATE_WITH_POLICY {
        let attrs = sqs
            .get_queue_attributes(GetQueueAttributesRequest {
                attribute_names: Some(vec!["All".to_string()]),
                queue_url: queue.url.clone(),
            })
            .await
            .unwrap()
            .attributes
            .unwrap();
        debug!("queue attributes: {:#?}", attrs);
        let queue_arn = attrs.get("QueueArn").unwrap();
        let mut attrs = HashMap::new();
        attrs.insert("Policy".to_string(), queue_policy(queue_arn));
        sqs.set_queue_attributes(SetQueueAttributesRequest {
            attributes: attrs,
            queue_url: queue.url.clone(),
        })
        .await
        .unwrap();
    }
    queue
}

pub struct Subscription {
    pub arn: String,
}

impl Drop for Subscription {
    fn drop(&mut self) {
        block_on(unsubscribe(&self.arn))
    }
}

pub async fn subscribe_queue(queue_name: &str) -> Subscription {
    let sns = rusoto_sns::SnsClient::new(REGION);
    let input = SubscribeInput {
        endpoint: Some(format!(
            "arn:aws:sqs:{}:{}:{}",
            REGION.name(),
            ACCOUNT_ID,
            queue_name
        )),
        topic_arn: format!(
            "arn:aws:sns:ap-southeast-1:{}:replica-search-events",
            ACCOUNT_ID
        ),
        protocol: "sqs".to_string(),
        ..Default::default()
    };
    Subscription {
        arn: sns
            .subscribe(input)
            .await
            .unwrap()
            .subscription_arn
            .unwrap(),
    }
}

pub async fn delete_queue(queue_url: &str) {
    let sqs = rusoto_sqs::SqsClient::new(REGION);
    sqs.delete_queue(DeleteQueueRequest {
        queue_url: queue_url.to_string(),
    })
    .await
    .unwrap();
    info!("deleted queue {}", queue_url);
}

pub async fn unsubscribe(arn: &str) {
    rusoto_sns::SnsClient::new(REGION)
        .unsubscribe(UnsubscribeInput {
            subscription_arn: arn.to_string(),
        })
        .await
        .unwrap();
    info!("unsubscribed {}", arn);
}
