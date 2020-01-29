use serde::{Deserialize, Serialize};

use crate::IndexState;

use crate::search::{self, OwnedMimeType};

use crate::bittorrent;
use log::*;

#[derive(Serialize)]
pub struct SearchResultItem {
    pub key: String,
    pub hits: usize,
}

type SearchResult = Vec<SearchResultItem>;

pub async fn search_response(index: &IndexState, query: impl Into<search::Query>) -> SearchResult {
    let index_query = query.into();
    let mut results: SearchResult = index
        .lock()
        .unwrap()
        .get_matches(&index_query)
        .into_iter()
        .map(|(key, hits)| SearchResultItem { key, hits })
        .collect();
    let query_value = index_query.terms.join(" ");
    match bittorrent::search(query_value.as_str()).await {
        Ok(more_results) => results.extend(more_results),
        Err(err) => error!("error searching bittorrent: {}", err),
    }
    results
}

#[deprecated(note = "search response result is now structured, use search_response instead")]
pub use search_response as search_response_body;

#[derive(Deserialize, Debug)]
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
