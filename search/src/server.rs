use serde::{Deserialize, Serialize};

use crate::IndexState;

#[derive(Serialize)]
pub struct SearchResultItem {
    key: String,
    hits: usize,
}

type SearchResult = Vec<SearchResultItem>;

pub fn search_response<I: AsRef<str>>(
    index: &IndexState,
    terms: impl Iterator<Item = I>,
    offset: Option<usize>,
    limit: Option<usize>,
) -> SearchResult {
    index
        .lock()
        .unwrap()
        .get_matches(terms, offset, limit)
        .into_iter()
        .map(|(key, hits)| SearchResultItem { key, hits })
        .collect()
}

#[deprecated(note = "search response result is now structured, use search_response instead")]
pub use search_response as search_response_body;

#[derive(Deserialize)]
pub struct SearchQuery {
    s: String,
    pub offset: Option<usize>,
    pub limit: Option<usize>,
}

impl SearchQuery {
    pub fn terms(&self) -> impl Iterator<Item = &str> {
        self.s.split_whitespace()
    }
}
