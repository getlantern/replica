use crate::s3::tokenize_object_key;
use crate::s3::*;

use log::*;
use std::sync::{Arc, Mutex};

use tokio::signal;
use uuid::Uuid;

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
    {
        let index = Arc::clone(&index);
        tokio::spawn(async move {
            let queue_name = format!("{}-{}", QUEUE_NAME_PREFIX, Uuid::new_v4().to_simple());
            let queue = create_event_queue(&queue_name).await;
            let subscription = subscribe_queue(&queue_name).await;
            info!("subscription arn: {}", subscription.0);
            add_all_objects(&index).await;
            receive_s3_events(&index, &queue.0).await;
        });
    }

    tokio::spawn(async move {
        run_server(index).await;
    });

    signal::ctrl_c().await.unwrap();
}

async fn add_all_objects(index: &Mutex<search::Index>) {
    let objects = get_all_objects().await;
    for obj in &objects {
        let key = obj.key.as_ref().unwrap();
        handle!(index.lock().unwrap().add_key(key), err, {
            error!("error adding {:?} to index: {}", key, err);
            continue;
        });
        info!("added {} to index", key);
    }
}
