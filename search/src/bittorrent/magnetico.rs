use super::SearchResultItem;
use anyhow::Context;
use anyhow::Result;
use log::*;
use reqwest::Url;
use serde::Deserialize;
use std::borrow::Borrow;
use std::collections::HashMap;
use std::sync::Arc;

#[derive(Deserialize, Copy, Clone)]
pub struct Epoch(pub i64);

#[derive(Deserialize)]
pub struct Torrent {
    #[serde(rename = "infoHash")]
    pub info_hash: String,
    pub id: u64,
    pub name: String,
    pub size: u64,
    #[serde(rename = "discoveredOn")]
    pub discovered_on: Epoch,
    #[serde(rename = "nFiles")]
    pub n_files: u64,
    pub relevance: f64,
}

#[derive(Deserialize, Clone, Debug)]
pub struct File {
    pub size: i64,
    pub path: String,
}

pub type Files = Vec<File>;

type ListFilesCache = HashMap<String, Files>;

pub struct Client {
    root_url: Url,
    http: reqwest::Client,
    list_files_singleflight: crate::singleflight::Group<String, Result<Files, Arc<anyhow::Error>>>,
    list_files_cache: tokio::sync::RwLock<ListFilesCache>,
}

cfg_if::cfg_if! {
    if #[cfg(feature="magnetico_localhost")] {
        const MAGNETICOW_LOCATION: &str = "http://localhost:8081";
        const MAGNETICO_USER: &str = "magnetico";
        const MAGNETICO_PASSWORD: Option<&str> = Some("magnetico");
    } else {
        const MAGNETICOW_LOCATION: &str = "http://replica.anacrolix.link:8080";
        const MAGNETICO_USER: &str = "derp";
        const MAGNETICO_PASSWORD: Option<&str> = Some("secret");
    }
}

impl Client {
    pub fn new() -> Self {
        Self {
            root_url: {
                let mut url = Url::parse(MAGNETICOW_LOCATION).unwrap();
                url.path_segments_mut().unwrap().extend(&["api", "v0.1"]);
                url
            },
            http: reqwest::Client::new(),
            list_files_singleflight: crate::singleflight::Group::new(),
            list_files_cache: Default::default(),
        }
    }

    // Holy crap look at this signature!
    async fn get<T, Q, K, V, P>(&self, path_segments: P, query_pairs: Q) -> Result<T>
    where
        T: serde::de::DeserializeOwned,
        Q: IntoIterator,
        Q::Item: Borrow<(K, V)>,
        K: AsRef<str>,
        V: AsRef<str>,
        P: IntoIterator,
        P::Item: AsRef<str>,
    {
        let mut url = self.root_url.clone();
        url.query_pairs_mut().extend_pairs(query_pairs);
        url.path_segments_mut().unwrap().extend(path_segments);
        let response = self
            .http
            .get(url)
            .basic_auth(MAGNETICO_USER, MAGNETICO_PASSWORD)
            .send()
            .await?;
        let status = response.status();
        response
            .json::<T>()
            .await
            .with_context(|| format!("status: {}", status))
    }

    pub async fn list_files(&self, info_hash: &str) -> Result<Files, Arc<anyhow::Error>> {
        {
            let cache = self.list_files_cache.read().await;
            if let Some(v) = cache.get(info_hash) {
                trace!("files for {} served from cache", info_hash);
                return Ok(v.clone());
            }
        }
        self.list_files_singleflight
            .work(&info_hash.to_string(), async {
                let res: Result<Files> = self
                    .get(
                        &["torrents", info_hash, "filelist"],
                        // Is there a nicer way to do this?
                        std::iter::empty::<(&str, &str)>(),
                    )
                    .await;
                match res {
                    Ok(files) => {
                        self.list_files_cache
                            .write()
                            .await
                            .insert(info_hash.to_string(), files.clone());
                        Ok(files)
                    }
                    Err(err) => Err(Arc::new(err)),
                }
            })
            .await
    }

    pub async fn search(&self, query: &str) -> Result<Vec<SearchResultItem>> {
        let torrents: Vec<Torrent> = self.get(&["torrents"], &[("query", query)]).await?;
        let mut ok = Vec::new();
        debug!("listing files for {} torrents", torrents.len());
        for (t, fs) in futures_util::future::join_all(torrents.into_iter().map(|t| async move {
            trace!("listing files for {}", &t.info_hash);
            let files = self.list_files(&t.info_hash).await;
            trace!("listing files for {} returned", &t.info_hash);
            (t, files)
        }))
        .await
        {
            match fs {
                Err(e) => error!("error getting files for {}: {}", t.info_hash, e),
                Ok(files) => {
                    trace!("torrent {} has {} files", t.info_hash, files.len());
                    ok.extend(files.into_iter().map(|f| SearchResultItem {
                        torrent_name: t.name.clone(),
                        info_hash: t.info_hash.clone(),
                        file_path: f.path,
                        size: f.size,
                        age: t.discovered_on.into(),
                    }))
                }
            }
        }
        Ok(ok)
    }
}
