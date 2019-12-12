use serde::{Deserialize, Serialize};

use crate::IndexState;

#[derive(Serialize)]
pub struct SearchResultItem {
    key: String,
}

type SearchResult = Vec<SearchResultItem>;

pub fn search_response<I: AsRef<str>>(
    index: &IndexState,
    terms: impl Iterator<Item = I>,
) -> SearchResult {
    index
        .lock()
        .unwrap()
        .get_matches(terms)
        .into_iter()
        .map(|key| SearchResultItem { key })
        .collect()
}

#[deprecated(note = "search response result is now structured, use search_response instead")]
pub use search_response as search_response_body;

#[derive(Deserialize)]
pub struct SearchQuery {
    s: String,
}

impl SearchQuery {
    pub fn terms(&self) -> impl Iterator<Item = &str> {
        self.s.split_whitespace()
    }
}
