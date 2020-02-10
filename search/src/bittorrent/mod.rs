mod magnetico;
pub use magnetico::*;

use crate::types::*;
use std::borrow::Borrow;

pub struct SearchResultItem {
    pub torrent_name: String,
    pub info_hash: String,
    pub file_path: String,
    pub size: FileSize,
    pub age: DateTime,
}

impl SearchResultItem {
    pub fn score<I>(&self, terms: I) -> usize
    where
        I: Iterator,
        I::Item: Borrow<NormalizedToken>,
    {
        let tokens: Vec<NormalizedToken> = [&self.torrent_name, &self.file_path]
            .iter()
            .map(|x| crate::search::split_name(x))
            .flatten()
            .map(NormalizedToken::new)
            .collect();
        let mut ok = 0;
        for t in terms {
            if tokens.contains(t.borrow()) {
                ok += 1;
            }
        }
        ok
    }
}
