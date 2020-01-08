use std::collections::hash_map::Entry;
use std::collections::hash_map::RandomState;
use std::collections::HashMap;
use std::collections::HashSet;

pub struct Index {
    // A map from normalized tokens to matching keys.
    terms: HashMap<String, HashSet<String>>,
    // All the keys in the index.
    keys: HashSet<String>,
    // The function used to tokenize keys.
    tokenize: Tokenizer,
    // Applied to all tokens coming in.
    normalize_token: TokenNormalizer,
    // This is used to maintain consistency between hashmaps built for search results.
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

    // Returns keys sorted by descending number of token matches. Offset and limit what you'd expect
    // in SQL.
    pub fn get_matches<I, K>(
        &self,
        tokens: I,
        offset: Option<usize>,
        limit: Option<usize>,
    ) -> Vec<(String, usize)>
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
        sortable
            .iter()
            .skip(offset.unwrap_or(0))
            .take(limit.unwrap_or(usize::max_value()))
            .map(|(k, hits)| ((**k).to_string(), **hits))
            .collect()
    }
}
