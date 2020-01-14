use crate::search::Index;

use rusoto_core::Region;
use rusoto_s3::*;
use rusoto_sns::*;
use rusoto_sqs::*;
use std::collections::HashMap;

use crate::handle;
use crate::STOP_ORDERING;
use log::*;
use serde_json::json;
use std::sync::Mutex;
use uuid::Uuid;

const REGION: Region = Region::ApSoutheast1;
const TEST_BOUNDARIES: bool = false;
const ACCOUNT_ID: &str = "670960738222";

pub fn get_all_objects() -> Vec<Object> {
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
        let mut list = match s3.list_objects_v2(req).sync() {
            Ok(ok) => ok,
            Err(err) => {
                eprintln!("error listing objects: {}", err);
                continue;
            }
        };
        let contents: &mut Vec<Object> = list.contents.as_mut().unwrap();
        all.extend(contents.drain(..));
        let next = list.next_continuation_token;
        match next {
            Some(_) => token = next,
            None => break,
        }
    }
    all
}

pub fn tokenize_object_key(key: &str) -> Result<Vec<String>, String> {
    if key.len() < 37 {
        return Err("key too short to be valid".to_string());
    }
    Uuid::parse_str(&key[..36]).map_err(|e| format!("parsing uuid: {}", e))?;
    let name = &key[37..];
    Ok(name
        .rsplitn(2, '.')
        .map(str::split_whitespace)
        .flatten()
        .map(ToString::to_string)
        .collect())
}

fn handle_event(event: &Event, index: &Mutex<Index>) -> Result<(), String> {
    (match event.r#type {
        EventType::Added => Index::add_key,
        EventType::Removed => Index::remove_key,
    })(&mut index.lock().unwrap(), event.key.as_str())
}

pub fn receive_s3_events(
    index: &Mutex<Index>,
    queue_url: &str,
    stop: &std::sync::atomic::AtomicBool,
) {
    let sqs = rusoto_sqs::SqsClient::new(REGION);
    loop {
        let input = rusoto_sqs::ReceiveMessageRequest {
            queue_url: queue_url.to_string(),
            // We use long-polling here, but wait for it to return before checking the stop flag.
            // Using None results in too many calls if the latency is low. TODO: Use the futures,
            // and do cancellation synchronously. Note that the maximum is Some(20).
            wait_time_seconds: Some(1),
            max_number_of_messages: Some(10),
            // visibility_timeout: Some(0),
            ..Default::default()
        };
        let result = sqs.receive_message(input).sync();
        trace!("receive_message returned");
        if stop.load(STOP_ORDERING) {
            trace!("got stop");
            return;
        }
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
                .sync();
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
}

use serde_json::Value as JsonValue;
use std::str::FromStr;

fn parse_record(rec: JsonValue) -> Result<Event, String> {
    Ok(Event {
        r#type: match rec["eventName"].as_str().unwrap() {
            "ObjectCreated:Put" => EventType::Added,
            "ObjectRemoved:Delete" => EventType::Removed,
            _ => return Err("unhandled event name".to_string()),
        },
        key: rec["s3"]["object"]["key"].as_str().unwrap().to_string(),
    })
}

fn get_records(body: String) -> Result<Vec<JsonValue>, String> {
    let value = JsonValue::from_str(body.as_str()).map_err(|e| format!("parsing json: {}", e))?;
    let mut value = JsonValue::from_str(value["Message"].as_str().unwrap())
        .map_err(|e| format!("parsing json in message field: {}", e))?;
    if let JsonValue::Array(records) = value["Records"].take() {
        Ok(records)
    } else {
        Err("shit fukt up".to_string())
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

pub fn create_event_queue(name: &str) -> String {
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
    let result = sqs.create_queue(input).sync().unwrap();
    let queue_url = result.queue_url.unwrap();
    if !CREATE_WITH_POLICY {
        let attrs = sqs
            .get_queue_attributes(GetQueueAttributesRequest {
                attribute_names: Some(vec!["All".to_string()]),
                queue_url: queue_url.clone(),
            })
            .sync()
            .unwrap()
            .attributes
            .unwrap();
        debug!("queue attributes: {:#?}", attrs);
        let queue_arn = attrs.get("QueueArn").unwrap();
        info!("created sqs queue {}", queue_url);
        let mut attrs = HashMap::new();
        attrs.insert("Policy".to_string(), queue_policy(queue_arn));
        sqs.set_queue_attributes(SetQueueAttributesRequest {
            attributes: attrs,
            queue_url: queue_url.clone(),
        })
        .sync()
        .unwrap();
    }
    queue_url
}

pub fn subscribe_queue(queue_name: &str) -> String {
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
    sns.subscribe(input)
        .sync()
        .unwrap()
        .subscription_arn
        .unwrap()
}

pub fn delete_queue(queue_url: &str) {
    let sqs = rusoto_sqs::SqsClient::new(REGION);
    sqs.delete_queue(DeleteQueueRequest {
        queue_url: queue_url.to_string(),
    })
    .sync()
    .unwrap();
    info!("deleted queue {}", queue_url);
}

pub fn unsubscribe(arn: String) {
    rusoto_sns::SnsClient::new(REGION)
        .unsubscribe(UnsubscribeInput {
            subscription_arn: arn.clone(),
        })
        .sync()
        .unwrap();
    info!("unsubscribed {}", arn);
}
