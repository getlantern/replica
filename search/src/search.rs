use std::collections::hash_map::Entry;
use std::collections::HashMap;
use std::collections::HashSet;

pub struct Index {
    terms: HashMap<String, HashSet<String>>,
    keys: HashSet<String>,
    tokenize: Tokenizer,
    normalize_token: TokenNormalizer,
}

type Tokenizer = &'static (dyn Fn(&str) -> Result<Vec<String>, String> + Send + Sync);

type TokenNormalizer = fn(&str) -> String;

impl Index {
    pub fn new(t: Tokenizer, tn: TokenNormalizer) -> Self {
        Self {
            tokenize: t,
            normalize_token: tn,
            keys: Default::default(),
            terms: Default::default(),
        }
    }

    pub fn add_key(&mut self, key: &str) -> Result<(), String> {
        for t in (self.tokenize)(key)? {
            let t = (self.normalize_token)(&t);
            self.terms.entry(t).or_default().insert(key.to_owned());
        }
        self.keys.insert(key.to_owned());
        Ok(())
    }

    pub fn remove_key(&mut self, key: &str) -> Result<(), String> {
        if !self.keys.remove(key) {
            return Err("key not in index".to_string());
        }
        for t in (self.tokenize)(key)? {
            let t = (self.normalize_token)(&t);
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

    pub fn get_matches<'a, I, K: 'a>(&self, tokens: I) -> Vec<String>
    where
        I: Iterator<Item = &'a K>,
        K: AsRef<str>,
    {
        let mut tokens = tokens.map(|x| (self.normalize_token)(x.as_ref()));
        let first = match tokens.next() {
            Some(i) => i,
            None => return Default::default(),
        };
        let mut all: HashSet<String> = self
            .terms
            .get(&first)
            .cloned()
            .unwrap_or(Default::default());
        for t in tokens {
            all = all
                .intersection(self.terms.get(&t).unwrap())
                .cloned()
                .collect();
        }
        all.into_iter().collect()
    }
}
