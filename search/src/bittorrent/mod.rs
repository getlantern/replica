use crate::server::SearchResultItem;
use anyhow::{Context, Result};

use reqwest::{Client, Method, Url};
mod magnetico;
use magnetico::Torrent;

pub async fn search(query: &str) -> Result<Vec<SearchResultItem>> {
    let client = Client::new();
    let url = Url::parse_with_params(
        "http://replica.anacrolix.link:8080/api/v0.1/torrents",
        &[("query", query)],
    )
    .unwrap();
    let response = client
        .request(Method::GET, url)
        .basic_auth("derp", Some("secret"))
        .send()
        .await?;
    let status = response.status();
    let result = response.json::<Vec<Torrent>>().await;
    match result {
        Ok(torrents) => Ok(torrents
            .into_iter()
            .map(|t| SearchResultItem {
                hits: 1,
                key: t.name,
            })
            .collect()),
        Err(err) => Err(anyhow::Error::new(err)),
    }
    .with_context(|| format!("status: {}", status))
}
