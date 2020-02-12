use crate::bittorrent;
use crate::search::{self, path_mime_types, OwnedMimeType};
use crate::types::*;
use log::*;
use serde::{Deserialize, Serialize};
use tokio::sync::Mutex;

#[derive(Serialize)]
pub struct SearchResultItem {
    pub replica_s3_key: Option<String>,
    pub search_term_hits: usize,
    pub info_hash: String,
    pub file_path: Option<String>,
    pub file_size: FileSize,
    pub torrent_name: Option<String>,
    pub mime_type: Option<String>,
    pub last_modified: crate::types::DateTime,
    pub replica_link: ReplicaLink,
}

impl SearchResultItem {
    fn from_bittorrent(
        t: bittorrent::SearchResultItem,
        terms: impl Iterator<Item = impl std::borrow::Borrow<NormalizedToken>>,
    ) -> Self {
        Self {
            mime_type: mime_guess::from_path(&t.file_path)
                .first()
                .map(|x| x.to_string()),
            search_term_hits: t.score(terms),
            info_hash: t.info_hash.clone(),
            file_path: Some(t.file_path.clone()),
            file_size: t.size,
            replica_s3_key: None,
            torrent_name: Some(t.torrent_name.clone()),
            last_modified: t.age,
            replica_link: ReplicaLink {
                info_hash: t.info_hash,
                display_name: Some(format!("{}/{}", t.torrent_name, t.file_path)),
                trackers: vec![],
                exact_source: None,
                acceptable_source: None,
            },
        }
    }
    fn from_search_index(t: search::SearchResultItem, search_type: Option<OwnedMimeType>) -> Self {
        Self {
            mime_type: search_type.or_else(|| {
                mime_guess::from_path(&t.s3_key)
                    .first()
                    .map(|x| x.to_string())
            }),
            replica_s3_key: Some(t.s3_key.clone()),
            search_term_hits: t.token_hits,
            info_hash: t.info_hash.to_string(),
            file_path: None,
            file_size: t.size,
            torrent_name: None,
            last_modified: t.last_modified,
            replica_link: ReplicaLink {
                exact_source: Some(format!("replica:{}", &t.s3_key)),
                acceptable_source: Some(format!(
                    "https://getlantern-replica.s3-ap-southeast-1.amazonaws.com/{}",
                    &t.s3_key
                )),
                info_hash: t.info_hash.to_string(),
                display_name: Some(t.s3_key),
                trackers: crate::replica::TRACKERS
                    .iter()
                    .map(ToString::to_string)
                    .collect(),
            },
        }
    }
}

type SearchResult = Vec<SearchResultItem>;

pub struct Server {
    pub bittorrent_search_client: bittorrent::Client,
    pub replica_s3_index: std::sync::Arc<Mutex<search::Index>>,
}

impl Server {
    pub async fn search_response(&self, query: &SearchQuery) -> SearchResult {
        let mut result: SearchResult = self
            .replica_s3_index
            .lock()
            .await
            .get_matches(query.terms(), &query.type_)
            .into_iter()
            .map(|x| SearchResultItem::from_search_index(x, query.type_.clone()))
            .collect();
        match self.bittorrent_search_client.search(&query.s).await {
            Ok(more_results) => result.extend(
                more_results
                    .into_iter()
                    .filter(|x| {
                        query.type_.as_ref().map_or(true, |query_type| {
                            path_mime_types(&x.file_path).any(|path_type| &path_type == query_type)
                        })
                    })
                    .map(|x| {
                        SearchResultItem::from_bittorrent(
                            x,
                            query.terms().map(NormalizedToken::new),
                        )
                    }),
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
