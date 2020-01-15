use serde::{Deserialize, Serialize};

use crate::IndexState;

use crate::search::{self, OwnedMimeType};

#[derive(Serialize)]
pub struct SearchResultItem {
    key: String,
    hits: usize,
}

type SearchResult = Vec<SearchResultItem>;

pub fn search_response(index: &IndexState, query: impl Into<search::Query>) -> SearchResult {
    index
        .lock()
        .unwrap()
        .get_matches(query.into())
        .into_iter()
        .map(|(key, hits)| SearchResultItem { key, hits })
        .collect()
}

#[deprecated(note = "search response result is now structured, use search_response instead")]
pub use search_response as search_response_body;

#[derive(Deserialize)]
pub struct SearchQuery {
    s: String,
    offset: Option<usize>,
    limit: Option<usize>,
    #[serde(rename = "type")]
    type_: Option<OwnedMimeType>,
}

impl SearchQuery {
    pub fn terms(&self) -> impl Iterator<Item = &str> {
        self.s.split_whitespace()
    }
}

impl Into<search::Query> for SearchQuery {
    fn into(self) -> search::Query {
        search::Query {
            terms: self.terms().map(|s| s.to_owned()).collect(),
            limit: self.limit,
            offset: self.offset,
            type_: self.type_,
        }
    }
}
