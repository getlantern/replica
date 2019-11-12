use failure::{ensure, Error};
use rusoto_core::Region;
use rusoto_s3::*;
use rusoto_sqs::Sqs;
use uuid::Uuid;
use human_size::{Byte,Kilobyte};

const REGION: Region = Region::ApSoutheast1;

fn main() {
    let objects = get_all_objects();
    for obj in &objects {
        let key = obj.key.as_ref().unwrap();
        println!(
            "{:>12} {} {:?}",
            format!("{:.1}",human_size::SpecificSize::new(obj.size.unwrap() as f64, Byte).unwrap().into::<Kilobyte>()),
            obj.key.as_ref().unwrap(),
            tokenize_object_key(key)
        );
    }
    receive_s3_events()
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
    Ok(key[37..]
        .split_whitespace()
        .map(ToString::to_string)
        .collect::<Vec<String>>())
}

fn receive_s3_events() {
    let sqs = rusoto_sqs::SqsClient::new(REGION);
    loop {
        let input = rusoto_sqs::ReceiveMessageRequest {
            queue_url: "https://sqs.ap-southeast-1.amazonaws.com/670960738222/replica-s3-events"
                .to_string(),
            // wait_time_seconds: Some(20),
            wait_time_seconds: Some(2),
            ..Default::default()
        };

        let result = sqs.receive_message(input).sync().unwrap();
        for msg in result.messages.unwrap_or_default() {
            let body = msg.body.unwrap();
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
