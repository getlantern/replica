use crate::search::Index;

use rusoto_core::Region;
use rusoto_s3::*;
use rusoto_sns::*;
use rusoto_sqs::*;
use std::collections::HashMap;

use crate::STOP_ORDERING;
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

pub fn tokenize_object_key(key: &str) -> std::result::Result<Vec<String>, String> {
    if key.len() < 37 {
        return Err(format!("key too short to be valid"));
    }
    Uuid::parse_str(&key[..36]).map_err(|e| format!("parsing uuid: {}", e))?;
    let name = &key[37..];
    Ok(name
        .rsplitn(2, '.')
        .map(str::split_whitespace)
        .flatten()
        .map(ToString::to_string)
        .collect::<Vec<String>>())
}

fn handle_event(name: &str, key: &str, index: &Mutex<Index>) -> std::result::Result<(), String> {
    match name {
        "ObjectCreated:Put" => {
            index.lock().unwrap().add_key(key)?;
            println!("added {} to index", key);
            Ok(())
        }
        "ObjectRemoved:Delete" => {
            index.lock().unwrap().remove_key(key)?;
            println!("removed {} from index", key);
            Ok(())
        }
        _ => Err(format!("unhandled event name: {}", name)),
    }
}

pub fn receive_s3_events(
    index: &Mutex<Index>,
    queue_url: &String,
    stop: &std::sync::atomic::AtomicBool,
) {
    let sqs = rusoto_sqs::SqsClient::new(REGION);
    loop {
        let input = rusoto_sqs::ReceiveMessageRequest {
            queue_url: queue_url.clone(),
            // We use long-polling here, but wait for it to return before checking the stop flag. 
            // TODO: Use the futures, and do cancellation synchronously. Note that the maximum is
            // Some(20).
            wait_time_seconds: Some(5),
            ..Default::default()
        };
        let result = sqs.receive_message(input).sync();
        if stop.load(STOP_ORDERING) {
            break;
        }
        let result = match result {
            Ok(ok) => ok,
            Err(err) => {
                eprintln!("error receiving messages: {}", err);
                continue;
            }
        };
        eprintln!("receive_message returned");
        for msg in result.messages.unwrap_or_default() {
            let body = msg.body.unwrap();
            // if let Err(err) = sqs
            //     .delete_message(rusoto_sqs::DeleteMessageRequest {
            //         queue_url: queue_url.to_owned(),
            //         receipt_handle: msg.receipt_handle.unwrap(),
            //     })
            //     .sync()
            // {
            //     eprintln!("error deleting message: {}", err);
            // }
            match serde_json::from_str::<serde_json::Value>(body.as_str()) {
                Err(e) => {
                    eprintln!("error parsing message body: {}", e);
                    continue;
                }
                Ok(value) => {
                    for obj in value["Records"].as_array().unwrap_or(&vec![]) {
                        if let Err(err) = handle_event(
                            obj["eventName"].as_str().unwrap(),
                            obj["s3"]["object"]["key"].as_str().unwrap(),
                            index,
                        ) {
                            eprintln!("error handling event: {}", err);
                        }
                    }
                }
            }
            // let event: aws_lambda_events::event::s3::S3Event =
            //     match serde_json::from_str(body.as_str()) {
            //         Ok(ok) => ok,
            //         Err(e) => {
            //             eprintln!("error parsing event: {:?} in {:?}", e, body);
            //             continue;
            //         }
            //     };
            // println!("{:#?}", event);
        }
    }
}

pub fn create_event_queue(name: &String) -> String {
    let sqs = rusoto_sqs::SqsClient::new(REGION);
    let mut attrs = HashMap::new();
    let policy = json!({
      "Version": "2012-10-17",
      "Id": format!("arn:aws:sqs:ap-southeast-1:{}:{}/SQSDefaultPolicy", ACCOUNT_ID,name),
      "Statement": [
        {
          "Sid": "SNSSend",
          "Effect": "Allow",
          "Principal": {
            "AWS": "*"
          },
          "Action": "SQS:SendMessage",
          "Resource": format!("arn:aws:sqs:ap-southeast-1:{}:{}", ACCOUNT_ID,name),
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
          "Resource": format!("arn:aws:sqs:ap-southeast-1:{}:{}", ACCOUNT_ID,name),
        }
      ]
    }).to_string();
    attrs.insert("Policy".to_string(), policy);
    let input = CreateQueueRequest {
        queue_name: name.clone(),
        attributes: Some(attrs),
        ..Default::default()
    };
    let result = sqs.create_queue(input).sync().unwrap();
    let queue_url = result.queue_url.unwrap();
    println!("created sqs queue {}", queue_url);
    // sqs.add_permission(AddPermissionRequest{
    //     aws_account_ids:vec![ACCOUNT_ID.to_string()],
    //     actions: vec!["SendMessage".to_string(), "ReceiveMessage".to_string()],
    //     label:Uuid::new_v4().to_string(),
    //     queue_url: queue_url.clone(),
    // }).sync().unwrap();
    queue_url
}

pub fn subscribe_queue(queue_name: &String) -> String {
    let sns = rusoto_sns::SnsClient::new(REGION);
    let input = SubscribeInput {
        endpoint: Some(
            format!(
                "arn:aws:sqs:{}:{}:{}",
                REGION.name(),
                ACCOUNT_ID,
                queue_name
            )
            .to_string(),
        ),
        topic_arn: format!(
            "arn:aws:sns:ap-southeast-1:{}:replica-search-events",
            ACCOUNT_ID
        )
        .to_string(),
        protocol: "sqs".to_string(),
        ..Default::default()
    };
    sns.subscribe(input)
        .sync()
        .unwrap()
        .subscription_arn
        .unwrap()
}

pub fn delete_queue(queue_url: &String) {
    let sqs = rusoto_sqs::SqsClient::new(REGION);
    sqs.delete_queue(DeleteQueueRequest {
        queue_url: queue_url.clone(),
    })
    .sync()
    .unwrap();
    println!("deleted queue {}", queue_url);
}

pub fn unsubscribe(arn: String) {
    rusoto_sns::SnsClient::new(REGION)
        .unsubscribe(UnsubscribeInput {
            subscription_arn: arn.clone(),
        })
        .sync()
        .unwrap();
    println!("unsubscribed {}", arn);
}
