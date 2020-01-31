use crate::s3::tokenize_object_key;
use crate::s3::*;

use log::*;
use std::sync::{Arc, Mutex};

use tokio::signal;
use uuid::Uuid;

pub use anyhow::Result;

mod bittorrent;
mod macros;
mod s3;
mod search;
mod server;
mod warp;
use crate::warp::run_server;

type IndexState = Arc<Mutex<search::Index>>;

const QUEUE_NAME_PREFIX: &str = "replica_search_queue";

#[tokio::main]
async fn main() {
    env_logger::init();
    let index = Arc::new(Mutex::new(search::Index::new(
        &tokenize_object_key,
        str::to_lowercase,
    )));
    let s3_index = Arc::clone(&index);
    tokio::select! {
        _ = s3_stuff(&s3_index) => {}
        _ = run_server(index) => {}
        r = signal::ctrl_c() => { r.unwrap() }
    }
}

async fn s3_stuff(index: &Mutex<search::Index>) {
    let queue_name = format!("{}-{}", QUEUE_NAME_PREFIX, Uuid::new_v4().to_simple());
    let queue = create_event_queue(&queue_name).await;
    let subscription = subscribe_queue(&queue_name).await;
    info!("subscription arn: {}", subscription.arn);
    add_all_objects(&index).await;
    receive_s3_events(&index, &queue.url).await;
}

async fn add_all_objects(index: &Mutex<search::Index>) {
    let objects = get_all_objects().await;
    for obj in &objects {
        trace!("adding s3 object {:?}", obj);
        let key = obj.key.as_ref().unwrap();
        handle!(
            index.lock().unwrap().add_key(
                key,
                search::KeyInfo {
                    size: obj.size.unwrap()
                }
            ),
            err,
            {
                error!("error adding {:?} to index: {}", key, err);
                continue;
            }
        );
        info!("added {} to index", key);
    }
}
