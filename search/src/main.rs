use failure::Error;
use rusoto_core::Region;
use rusoto_s3::*;
use std::convert::TryFrom;
use uuid::Uuid;

fn main() {
    let objects = get_all_objects();
    for obj in &objects {
        let key = obj.key.as_ref().unwrap();
        println!(
            "{:>10} {} {:?}",
            bytesize::ByteSize(u64::try_from(obj.size.unwrap()).unwrap()).to_string(),
            obj.key.as_ref().unwrap(),
            tokenize_object_key(key)
        );
    }
}

fn get_all_objects() -> Vec<Object> {
    let s3 = S3Client::new(Region::ApSoutheast1);
    let mut all: Vec<Object> = vec![];
    let mut token = None;
    loop {
        let req = ListObjectsV2Request {
            bucket: "getlantern-replica".to_string(),
            // max_keys: Some(30),
            continuation_token: token,
            ..Default::default()
        };
        let mut list = s3.list_objects_v2(req).sync().unwrap();
        let contents: &mut Vec<Object> = list.contents.as_mut().unwrap();
        all.append(contents);
        let next = list.next_continuation_token;
        match next {
            Some(_) => token = next,
            None => break,
        }
    }
    all
}

fn tokenize_object_key(key: &str) -> Result<Vec<String>, Error> {
    let _uuid = Uuid::parse_str(&key[..36])?;
    Ok(key[37..]
        .split_whitespace()
        .map(ToString::to_string)
        .collect::<Vec<String>>())
}
