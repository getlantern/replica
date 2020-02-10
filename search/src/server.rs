use serde::{Deserialize, Serialize};

use crate::IndexState;

use crate::search::{self, OwnedMimeType};

use crate::bittorrent;
use crate::types::*;
use log::*;

#[derive(Serialize)]
pub struct SearchResultItem {
    pub replica_s3_key: Option<String>,
    pub search_term_hits: usize,
    pub info_hash: Option<String>,
    pub file_path: Option<String>,
    pub file_size: FileSize,
    pub torrent_name: Option<String>,
    pub mime_type: Option<String>,
    pub last_modified: crate::types::DateTime,
}

impl SearchResultItem {
    fn from_bittorrent<'a>(
        t: bittorrent::SearchResultItem,
        terms: impl Iterator<Item = impl std::borrow::Borrow<NormalizedToken>>,
    ) -> Self {
        Self {
            mime_type: mime_guess::from_path(&t.file_path)
                .first()
                .map(|x| x.to_string()),
            search_term_hits: t.score(terms),
            info_hash: Some(t.info_hash),
            file_path: Some(t.file_path),
            file_size: t.size,
            replica_s3_key: None,
            torrent_name: Some(t.torrent_name),
            last_modified: t.age,
        }
    }
    fn from_search_index(t: search::SearchResultItem, search_type: Option<OwnedMimeType>) -> Self {
        Self {
            mime_type: search_type.or_else(|| {
                mime_guess::from_path(&t.s3_key)
                    .first()
                    .map(|x| x.to_string())
            }),
            replica_s3_key: Some(t.s3_key),
            search_term_hits: t.token_hits,
            info_hash: None,
            file_path: None,
            file_size: t.size,
            torrent_name: None,
            last_modified: t.last_modified,
        }
    }
}

type SearchResult = Vec<SearchResultItem>;

pub struct Server {
    pub bittorrent_search_client: bittorrent::Client,
    pub replica_s3_index: IndexState,
}

impl Server {
    pub async fn search_response(&self, query: &SearchQuery) -> SearchResult {
        let mut result: SearchResult = self
            .replica_s3_index
            .lock()
            .unwrap()
            .get_matches(query.terms(), &query.type_)
            .into_iter()
            .map(|x| SearchResultItem::from_search_index(x, query.type_.clone()))
            .collect();
        match self.bittorrent_search_client.search(&query.s).await {
            Ok(more_results) => result.extend(more_results.into_iter().map(|x| {
                SearchResultItem::from_bittorrent(x, query.terms().map(NormalizedToken::new))
            })),
            Err(err) => error!("error searching bittorrent: {}", err),
        }
        result.sort_by(|l, r| l.search_term_hits.cmp(&r.search_term_hits).reverse());
        result
            .into_iter()
            .skip(query.offset.unwrap_or(0))
            .take(query.limit.unwrap_or(20))
            .collect()
    }
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
