#![feature(proc_macro_hygiene, decl_macro)]
extern crate rocket;
use failure::{ensure, Error};
use human_size::{Byte, Kilobyte};
use rusoto_core::Region;
use rusoto_s3::*;
use rusoto_sqs::Sqs;
use uuid::Uuid;
mod search;
mod server;

const REGION: Region = Region::ApSoutheast1;

fn main() {
    let mut index: search::Index = Default::default();
    let objects = get_all_objects();
    for obj in &objects {
        let key = obj.key.as_ref().unwrap();
        if let Err(err) = index.add_key(key) {
            eprintln!("error adding {:?} to index: {}", key, err)
        }
        println!(
            "{:>12} {} {:?}",
            // Only handles the precision flag, so we have to wrap it with another format.
            format!(
                "{:.1}",
                human_size::SpecificSize::new(obj.size.unwrap() as f64, Byte)
                    .unwrap()
                    .into::<Kilobyte>()
            ),
            obj.key.as_ref().unwrap(),
            tokenize_object_key(key)
        );
    }
    std::thread::spawn(receive_s3_events);
    server::run_server(index);
}

#[test]
fn test_human_byte_size_ignores_padding() {
    // When this fails, maybe human_size handles padding.
    assert_eq!(
        format!("{:5}", human_size::SpecificSize::new(1, Byte).unwrap()),
        "1 B"
    )
}

fn get_all_objects() -> Vec<Object> {
    let s3 = S3Client::new(REGION);
    let mut all: Vec<Object> = vec![];
    let mut token = None;
    loop {
        let req = ListObjectsV2Request {
            bucket: "getlantern-replica".to_string(),
            max_keys: Some(2),
            continuation_token: token,
            ..Default::default()
        };
        let mut list = s3.list_objects_v2(req).sync().unwrap();
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

fn tokenize_object_key(key: &str) -> Result<Vec<String>, Error> {
    ensure!(key.len() > 37);
    Uuid::parse_str(&key[..36])?;
    let name = &key[37..];
    Ok(name
        .rsplitn(2, '.')
        .map(str::split_whitespace)
        .flatten()
        .map(ToString::to_string)
        .collect::<Vec<String>>())
}

const QUEUE_URL: &str = "https://sqs.ap-southeast-1.amazonaws.com/670960738222/replica-s3-events";

fn receive_s3_events() {
    let sqs = rusoto_sqs::SqsClient::new(REGION);
    loop {
        let input = rusoto_sqs::ReceiveMessageRequest {
            queue_url: QUEUE_URL.to_string(),
            // wait_time_seconds: Some(20),
            wait_time_seconds: Some(2),
            ..Default::default()
        };

        let result = sqs.receive_message(input).sync().unwrap();
        for msg in result.messages.unwrap_or_default() {
            let body = msg.body.unwrap();
            if let Err(err) = sqs
                .delete_message(rusoto_sqs::DeleteMessageRequest {
                    queue_url: QUEUE_URL.to_owned(),
                    receipt_handle: msg.receipt_handle.unwrap(),
                })
                .sync()
            {
                eprintln!("error deleting message: {}", err);
            }
            let event: aws_lambda_events::event::s3::S3Event =
                match serde_json::from_str(body.as_str()) {
                    Ok(ok) => ok,
                    Err(e) => {
                        eprintln!("error parsing event: {:?} in {:?}", e, body);
                        continue;
                    }
                };
            println!("{:#?}", event);
        }
    }
}
