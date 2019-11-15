use crate::search::Index;
use failure::ensure;

use rusoto_core::Region;
use rusoto_s3::*;
use rusoto_sqs::Sqs;
use std::error::Error;
use std::sync::Mutex;
use uuid::Uuid;

const REGION: Region = Region::ApSoutheast1;

pub fn get_all_objects() -> Vec<Object> {
    let s3 = S3Client::new(REGION);
    let mut all: Vec<Object> = vec![];
    let mut token = None;
    loop {
        let req = ListObjectsV2Request {
            bucket: "getlantern-replica".to_string(),
            max_keys: Some(2),
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

const QUEUE_URL: &str = "https://sqs.ap-southeast-1.amazonaws.com/670960738222/replica-s3-events";

fn handle_event(name: &str, key: &str, index: &Mutex<Index>) -> std::result::Result<(), String> {
    match name {
        "ObjectCreated:Put" => index.lock().unwrap().add_key(key),
        "ObjectRemoved:Delete" => index.lock().unwrap().remove_key(key),
        _ => Err(format!("unhandled event name: {}", name)),
    }
}

pub fn receive_s3_events(index: &Mutex<Index>) {
    let sqs = rusoto_sqs::SqsClient::new(REGION);
    loop {
        let input = rusoto_sqs::ReceiveMessageRequest {
            queue_url: QUEUE_URL.to_string(),
            // wait_time_seconds: Some(20),
            wait_time_seconds: Some(2),
            ..Default::default()
        };

        let result = match sqs.receive_message(input).sync() {
            Ok(ok) => ok,
            Err(err) => {
                eprintln!("error receiving messages: {}", err);
                continue;
            }
        };
        for msg in result.messages.unwrap_or_default() {
            let body = msg.body.unwrap();
            // if let Err(err) = sqs
            //     .delete_message(rusoto_sqs::DeleteMessageRequest {
            //         queue_url: QUEUE_URL.to_owned(),
            //         receipt_handle: msg.receipt_handle.unwrap(),
            //     })
            //     .sync()
            // {
            //     eprintln!("error deleting message: {}", err);
            // }
            let value: serde_json::Value = serde_json::from_str(body.as_str()).unwrap();
            println!("got message: {:#?}", value);
            for obj in value["Records"].as_array().unwrap() {
                if let Err(err) = handle_event(
                    obj["eventName"].as_str().unwrap(),
                    obj["s3"]["object"]["key"].as_str().unwrap(),
                    index,
                ) {
                    eprintln!("error handling event: {}", err);
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
