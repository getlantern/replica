use serde::{Deserialize, Serialize};

use crate::IndexState;

use crate::search::{self, OwnedMimeType};

use crate::bittorrent;
use log::*;

#[derive(Serialize)]
pub struct SearchResultItem {
    pub replica_s3_key: Option<String>,
    pub search_term_hits: usize,
    pub info_hash: Option<String>,
    pub file_path: Option<String>,
    pub file_size: bittorrent::FileSize,
    pub torrent_name: Option<String>,
}

impl SearchResultItem {
    fn from<'a>(t: bittorrent::SearchResultItem, terms: impl Iterator<Item = &'a str>) -> Self {
        Self {
            search_term_hits: t.score(terms),
            info_hash: Some(t.info_hash),
            file_path: Some(t.file_path),
            file_size: t.size,
            replica_s3_key: None,
            torrent_name: Some(t.torrent_name),
        }
    }
}

impl From<search::SearchResultItem> for SearchResultItem {
    fn from(t: search::SearchResultItem) -> Self {
        Self {
            replica_s3_key: Some(t.s3_key),
            search_term_hits: t.token_hits,
            info_hash: None,
            file_path: None,
            file_size: t.size,
            torrent_name: None,
        }
    }
}

type SearchResult = Vec<SearchResultItem>;

pub async fn search_response(index: &IndexState, query: &SearchQuery) -> SearchResult {
    let mut result: SearchResult = index
        .lock()
        .unwrap()
        .get_matches(query.terms(), &query.type_)
        .into_iter()
        .map(Into::into)
        .collect();
    match bittorrent::Client::new().search(&query.s).await {
        Ok(more_results) => result.extend(
            more_results
                .into_iter()
                .map(|x| SearchResultItem::from(x, query.terms())),
        ),
        Err(err) => error!("error searching bittorrent: {}", err),
    }
    result.sort_by(|l, r| l.search_term_hits.cmp(&r.search_term_hits).reverse());
    result
        .into_iter()
        .skip(query.offset.unwrap_or(0))
        .take(query.limit.unwrap_or(20))
        .collect()
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
