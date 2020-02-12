use crate::s3::tokenize_object_key;
use crate::s3::*;
use ::search::*;
use log::*;
use std::sync::{Arc};
use tokio::sync::Mutex;
use tokio::signal;
use uuid::Uuid;
use ::search::types::*;
use futures::future::join_all;
use tokio::spawn;

const QUEUE_NAME_PREFIX: &str = "replica_search_queue";

#[tokio::main]
async fn main() {
    env_logger::init();
    let index = Arc::new(Mutex::new(search::Index::new(
        &tokenize_object_key,
        NormalizedToken::new,
    )));
    let s3_index = Arc::clone(&index);
    let server = server::Server {
        replica_s3_index: index,
        bittorrent_search_client: bittorrent::Client::new(),
    };
    tokio::select! {
        _ = s3_stuff(s3_index) => {}
        _ = ::search::warp::run_server(Arc::new(server)) => {}
        r = signal::ctrl_c() => { r.unwrap() }
    }
}

async fn s3_stuff(index: Arc<Mutex<search::Index>>) {
    let queue_name = format!("{}-{}", QUEUE_NAME_PREFIX, Uuid::new_v4().to_simple());
    let queue = create_event_queue(&queue_name).await;
    let subscription = subscribe_queue(&queue_name).await;
    info!("created subscription {}", subscription);
    add_all_objects(index.clone()).await;
    trace!("processing s3 events");
    receive_s3_events(&index, &queue.url).await;
}

async fn add_all_objects(index: Arc<Mutex<search::Index>>) {
    let objects = get_all_objects().await;
    let mut tasks = vec![];
    for obj in objects {
        trace!("adding s3 object {:?}", obj);
        let index = index.clone();
        tasks.push(spawn(async move {
            let key = obj.key.as_ref().unwrap();
            // TODO: This could fail.
            let info_hash = s3::get_infohash(key.to_string()).await.unwrap();
            if let Err(err) = index.lock().await.add_key(
                key,
                search::KeyInfo {
                    info_hash,
                    size: obj.size.unwrap(),
                    last_modified: {
                        let t = obj.last_modified.as_ref().unwrap();
                        debug!("s3 object {} last modified {}", key, t);
                        // 2020-01-15T01:24:23.000Z
                        handle!(DateTime::parse_from_s3(t), err, {
                            error!("error parsing time {:?}: {}", t, err);
                            DateTime::now()
                        })
                    },
                },
            ) {
                error!("error adding {:?} to index: {}", key, err);
                return;
            }
            info!("added {} to index", key);
        }));
    }
    join_all(tasks).await;
}
