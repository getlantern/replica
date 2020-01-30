use serde::{Deserialize, Serialize};

use crate::IndexState;

use crate::search::{self, OwnedMimeType};

use crate::bittorrent;
use log::*;

#[derive(Serialize, Default)]
pub struct SearchResultItem {
    pub key: Option<String>,
    pub hits: usize,
    pub info_hash: Option<String>,
    pub file_path: Option<String>,
    pub size: Option<bittorrent::FileSize>,
}

impl From<bittorrent::SearchResultItem> for SearchResultItem {
    fn from(t: bittorrent::SearchResultItem) -> Self {
        Self {
            info_hash: Some(t.info_hash),
            file_path: Some(t.file_path),
            size: Some(t.size),
            ..Default::default()
        }
    }
}

type SearchResult = Vec<SearchResultItem>;

pub async fn search_response(index: &IndexState, query: impl Into<search::Query>) -> SearchResult {
    let index_query = query.into();
    let mut results: SearchResult = index
        .lock()
        .unwrap()
        .get_matches(&index_query)
        .into_iter()
        .map(|(key, hits)| SearchResultItem {
            key: Some(key),
            hits,
            ..Default::default()
        })
        .collect();
    let query_value = index_query.terms.join(" ");
    match bittorrent::Client::new().search(query_value.as_str()).await {
        Ok(more_results) => results.extend(more_results.into_iter().map(Into::into)),
        Err(err) => error!("error searching bittorrent: {}", err),
    }
    results
}

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
