#[macro_use(defer)]
extern crate scopeguard;

use crate::s3::tokenize_object_key;
use crate::s3::*;

use anyhow::Result;
use log::*;
use std::sync::{
    atomic::*,
    mpsc::{channel, Sender},
    Arc, Mutex,
};
use std::thread::*;
use uuid::Uuid;

mod actix;
mod macros;
// mod hyper;
mod bittorrent;
mod s3;
mod search;
mod server;
use actix::run_server;

type IndexState = Arc<Mutex<search::Index>>;

const QUEUE_NAME_PREFIX: &str = "replica_search_queue";

pub const STOP_ORDERING: Ordering = Ordering::Relaxed;

fn main() {
    env_logger::init();
    // Any message on here triggers termination.
    let (tx, rx) = channel();
    {
        let tx = tx.clone();
        ctrlc::set_handler(move || {
            debug!("handling ctrlc");
            tx.send(()).unwrap();
        })
        .unwrap();
    }
    let vital_threads = VitalThreads {
        index: &Arc::new(Mutex::new(search::Index::new(
            &tokenize_object_key,
            str::to_lowercase,
        ))),
        tx: &tx,
        stop: Arc::new(AtomicBool::new(false)),
    };
    let s3_thread_join_handle = vital_threads.spawn(move |index, stop| {
        let queue_name = format!("{}-{}", QUEUE_NAME_PREFIX, Uuid::new_v4().to_simple());
        let queue_url = create_event_queue(&queue_name);
        defer! {delete_queue(&queue_url)};
        let subscription_arn = subscribe_queue(&queue_name);
        info!("subscription arn: {}", subscription_arn);
        defer!(unsubscribe(subscription_arn));
        add_all_objects(&index);
        receive_s3_events(&index, &queue_url, &stop);
    });
    vital_threads.spawn(move |index, _| run_server(index));
    rx.recv().unwrap();
    vital_threads.stop.store(true, STOP_ORDERING);
    s3_thread_join_handle.join().unwrap();
}

// Wraps tasks that take a search index and stop flag, and signal the stop flag when they fail.
struct VitalThreads<'a> {
    index: &'a Arc<Mutex<search::Index>>,
    tx: &'a Sender<()>,
    stop: Arc<AtomicBool>,
}

impl VitalThreads<'_> {
    fn spawn<F>(&self, f: F) -> JoinHandle<()>
    where
        F: FnOnce(Arc<Mutex<search::Index>>, Arc<AtomicBool>) -> (),
        F: Send + 'static,
    {
        let index = Arc::clone(self.index);
        let tx = self.tx.clone();
        let stop = Arc::clone(&self.stop);
        spawn(move || {
            defer!(tx.send(()).unwrap());
            f(index, stop);
        })
    }
}

fn add_all_objects(index: &Mutex<search::Index>) {
    let objects = get_all_objects();
    for obj in &objects {
        let key = obj.key.as_ref().unwrap();
        handle!(index.lock().unwrap().add_key(key), err, {
            error!("error adding {:?} to index: {}", key, err);
            continue;
        });
        info!("added {} to index", key);
    }
}
