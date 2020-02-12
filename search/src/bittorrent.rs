mod magnetico;
use crate::types::*;
pub use magnetico::*;
use std::borrow::Borrow;
use std::collections::HashSet;

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
        let token_groups: Vec<HashSet<NormalizedToken>> = [&self.torrent_name, &self.file_path]
            .iter()
            .map(|x| {
                crate::search::split_name(x)
                    .map(NormalizedToken::new)
                    .collect()
            })
            .collect();
        let mut ok = 0;
        for t in terms {
            for g in &token_groups {
                if g.contains(t.borrow()) {
                    ok += 1;
                }
            }
        }
        ok
    }
}
