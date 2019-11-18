use crate::s3::tokenize_object_key;
use failure::Error;
use std::collections::HashMap;

use std::collections::hash_map::Entry;
use std::collections::HashSet;

#[derive(Default)]
pub struct Index {
    pub terms: HashMap<String, HashSet<String>>,
    pub keys: HashSet<String>,
}

impl Index {
    pub fn add_key(&mut self, key: &str) -> Result<(), String> {
        for t in crate::tokenize_object_key(key).map_err(|e| e.to_string())? {
            self.terms.entry(t).or_default().insert(key.to_owned());
        }
        self.keys.insert(key.to_owned());
        Ok(())
    }

    pub fn remove_key(&mut self, key: &str) -> Result<(), String> {
        if !self.keys.remove(key) {
            return Err("key not in index".to_string());
        }
        for t in tokenize_object_key(key)? {
            if let Entry::Occupied(mut e) = self.terms.entry(t) {
                let v = e.get_mut();
                assert!(v.remove(key));
                if v.is_empty() {
                    e.remove();
                }
            } else {
                panic!();
            }
        }
        Ok(())
    }

    pub fn get_matches<'a, I, K: 'a>(&self, mut tokens: I) -> Vec<String>
    where
        I: Iterator<Item = &'a K>,
        String: std::borrow::Borrow<K>,
        K: std::hash::Hash + std::cmp::Eq,
    {
        let first = match tokens.next() {
            Some(i) => i,
            None => return Default::default(),
        };
        let mut all: HashSet<String> = self.terms.get(first).cloned().unwrap_or(Default::default());
        for t in tokens {
            all = all
                .intersection(self.terms.get(&t).unwrap())
                .cloned()
                .collect();
        }
        all.into_iter().collect()
    }
}
