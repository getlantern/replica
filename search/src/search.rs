use std::collections::hash_map::Entry;
use std::collections::hash_map::RandomState;
use std::collections::HashMap;
use std::collections::HashSet;

pub struct Index {
    terms: HashMap<String, HashSet<String>>,
    keys: HashSet<String>,
    tokenize: Tokenizer,
    normalize_token: TokenNormalizer,
    scores_random_state: RandomState,
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
            scores_random_state: Default::default(),
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

    pub fn get_matches<I, K>(&self, tokens: I) -> Vec<String>
    where
        I: Iterator<Item = K>,
        K: AsRef<str>,
    {
        let tokens = tokens.map(|x| (self.normalize_token)(x.as_ref()));
        let mut scores = HashMap::with_hasher(self.scores_random_state.clone());
        scores.extend(self.keys.iter().map(|k| (k.as_str(), 0)));
        for token in tokens {
            for key in self.terms.get(&token).into_iter().flatten() {
                *scores.entry(key).or_default() += 1;
            }
        }
        let mut sortable = scores.iter().collect::<Vec<_>>();
        sortable.sort_by(|(_, vl), (_, vr)| vl.cmp(vr).reverse());
        sortable.iter().map(|(k, _v)| k.to_string()).collect()
    }
}
