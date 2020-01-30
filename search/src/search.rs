use crate::Result;
use anyhow::*;
use std::collections::{
    hash_map::{Entry, RandomState},
    HashMap, HashSet,
};

pub type OwnedMimeType = String;

pub struct Index {
    // A map from normalized tokens to matching keys.
    terms: HashMap<String, HashSet<String>>,
    // All the keys in the index.
    all_keys: HashSet<String>,
    // Keys by their MIME top-level types.
    keys_by_type: HashMap<OwnedMimeType, HashSet<String>>,
    // The function used to tokenize keys.
    tokenize: Tokenizer,
    // Applied to all tokens coming in.
    normalize_token: TokenNormalizer,
    // This is used to maintain consistency between hashmaps built for search results.
    scores_random_state: RandomState,
}

type Tokenizer = &'static (dyn Fn(&str) -> Result<Vec<String>> + Send + Sync);

type TokenNormalizer = fn(&str) -> String;

pub struct Query {
    pub terms: Vec<String>,
    pub offset: Option<usize>,
    pub limit: Option<usize>,
    pub type_: Option<OwnedMimeType>,
}

impl Index {
    pub fn new(t: Tokenizer, tn: TokenNormalizer) -> Self {
        Self {
            tokenize: t,
            normalize_token: tn,
            all_keys: Default::default(),
            terms: Default::default(),
            scores_random_state: Default::default(),
            keys_by_type: Default::default(),
        }
    }

    // TODO: Should this belong on a Key type?
    fn key_mime_types(key: &str) -> impl Iterator<Item = OwnedMimeType> {
        mime_guess::from_path(key)
            .iter()
            .map(|guess| guess.type_().to_string())
    }

    pub fn normalized_tokens(&self, s: &str) -> Result<Vec<String>> {
        Ok((self.tokenize)(s)?
            .iter()
            .map(|t| (self.normalize_token)(&t))
            .collect())
    }

    pub fn add_key(&mut self, key: &str) -> Result<()> {
        for t in self.normalized_tokens(key)? {
            self.terms.entry(t).or_default().insert(key.to_owned());
        }
        self.all_keys.insert(key.to_owned());
        for type_ in Index::key_mime_types(key) {
            self.keys_by_type
                .entry(type_)
                .or_default()
                .insert(key.to_string());
        }
        Ok(())
    }

    pub fn remove_key(&mut self, key: &str) -> Result<()> {
        ensure!(self.all_keys.remove(key), "key not in index");
        for t in self.normalized_tokens(key)? {
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
        for type_ in Index::key_mime_types(key) {
            // We know the top-level MIME type has to be present, but we don't know (or care) how
            // many guesses match it. There's no point removing the set if we empty it, as the types
            // are quite coarse.
            self.keys_by_type
                .get_mut(type_.as_str())
                .unwrap()
                .remove(key);
        }
        Ok(())
    }

    // Returns keys sorted by descending number of token matches. Offset and limit what you'd expect
    // in SQL.
    pub fn get_matches(
        &self,
        terms: impl Iterator<Item = impl AsRef<str>>,
        type_: &Option<OwnedMimeType>,
    ) -> Vec<SearchResultItem> {
        let tokens = terms.map(|x| (self.normalize_token)(x.as_ref()));
        // Reuse the hasher state, to ensure stable search results (results at the same rank are
        // always ordered the same).
        let mut scores = HashMap::with_hasher(self.scores_random_state.clone());
        // Initialize scores for keys matching the search type.
        let keys: Box<dyn Iterator<Item = &String>> = match type_ {
            None => Box::new(self.all_keys.iter()),
            Some(t) => Box::new(self.keys_by_type.get(t).into_iter().flatten()),
        };
        scores.extend(keys.map(|k| (k.as_str(), 0)));
        // Score keys for the number of matching tokens.
        for token in tokens {
            for key in self.terms.get(&token).into_iter().flatten() {
                scores.entry(key).and_modify(|score| *score += 1);
            }
        }
        scores
            .iter()
            .map(|(s3_key, token_hits)| SearchResultItem {
                s3_key: (*s3_key).to_string(),
                token_hits: *token_hits,
            })
            .collect()
    }
}

pub struct SearchResultItem {
    pub s3_key: String,
    pub token_hits: usize,
}

pub fn split_name(s: &str) -> impl Iterator<Item = &str> {
    s.split(|c: char| c.is_whitespace() || c.is_ascii_punctuation())
}
