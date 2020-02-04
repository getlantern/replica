use core::future::Future;
use std::collections::HashMap;
use std::default::Default;
use std::fmt::Debug;
use std::hash::Hash;
use tokio::sync::watch;
use tokio::sync::Mutex;

#[derive(Default)]
pub struct Group<K: Hash + Eq, V: Clone + Debug> {
    pending: Mutex<HashMap<K, watch::Receiver<Option<V>>>>,
}

// I'm sure there's a way to wrap these up so the Option and channel mechanics aren't hanging out.
type PendingReceiver<V> = watch::Receiver<Option<V>>;
type PendingSender<V> = watch::Sender<Option<V>>;

enum GetPending<V> {
    AlreadyPending(PendingReceiver<V>),
    NewlyPending(PendingSender<V>),
}

use GetPending::*;

impl<K: Hash + Eq + Clone, V: Clone + Debug> Group<K, V> {
    pub async fn work(&self, key: &K, f: impl Future<Output = V>) -> V {
        match {
            let mut pending = self.pending.lock().await;
            match pending.get_mut(&key) {
                // Return a new receiver for the pending value.
                Some(rx) => AlreadyPending(rx.clone()),
                None => {
                    // Create a new broadcast pair.
                    let (tx, rx) = watch::channel(None);
                    pending.insert(key.clone(), rx);
                    NewlyPending(tx)
                }
            }
        } {
            AlreadyPending(mut rx) => loop {
                // Wait until a value is present in the receiver.
                if let Some(v) = rx.recv().await.unwrap() {
                    return v;
                }
            },
            NewlyPending(tx) => {
                // Do the work, lock the waiters, broadcast the work result to them, then remove
                // that we're doing the work.
                let v = f.await;
                let mut pending = self.pending.lock().await;
                tx.broadcast(Some(v.clone())).unwrap();
                pending.remove(&key);
                v
            }
        }
    }
    pub fn new() -> Self {
        Self {
            // Can't do this with Default for some reason.
            pending: Mutex::new(HashMap::new()),
        }
    }
}
