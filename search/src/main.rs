#![feature(proc_macro_hygiene, decl_macro)]
extern crate rocket;
#[macro_use(defer)]
extern crate scopeguard;

use crate::s3::tokenize_object_key;
use crate::s3::*;
use human_size::{Byte, Kilobyte};

use std::sync::{Arc, Mutex};
use uuid::Uuid;

mod s3;
mod search;
mod server;

const QUEUE_NAME_PREFIX: &'static str = "replica_search_queue";

fn main() {
    let index = Arc::new(Mutex::new(search::Index::default()));
    let mut threads = vec![];
    {
        let index = Arc::clone(&index);
        threads.push(std::thread::spawn(move || {
            let queue_name = format!("{}-{}", QUEUE_NAME_PREFIX, Uuid::new_v4().to_simple());
            let queue_url = create_event_queue(&queue_name);
            defer! {{delete_queue(&queue_url)}};
            let subscription_arn = subscribe_queue(&queue_name);
            defer!({unsubscribe(subscription_arn)});
            add_all_objects(&index);
            receive_s3_events(&index, &queue_url);
        }));
    }
    threads.push(std::thread::spawn(|| server::run_server(index)));
    for t in threads.into_iter().rev() {
        t.join().unwrap();
    }
}

fn add_all_objects(index: &Mutex<search::Index>) {
    let objects = get_all_objects();
    for obj in &objects {
        let key = obj.key.as_ref().unwrap();
        if let Err(err) = index.lock().unwrap().add_key(key) {
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
}

#[test]
fn test_human_byte_size_ignores_padding() {
    // When this fails, maybe human_size handles padding.
    assert_eq!(
        format!("{:5}", human_size::SpecificSize::new(1, Byte).unwrap()),
        "1 B"
    )
}
