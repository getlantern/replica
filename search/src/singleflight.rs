use std::collections::HashMap;
use tokio::sync::watch;

pub struct Group<K, V> {
    work: HashMap<K, watch::Receiver<V>>
}