use crate::server::SearchResultItem;
use anyhow::Result;
use log::*;
use reqwest::{blocking::Client, Method, Url};
mod magnetico;
use magnetico::Torrent;

pub fn search(query: &str) -> Result<Vec<SearchResultItem>> {
    let client = Client::new();
    let url = Url::parse_with_params(
        "http://replica.anacrolix.link:8080/api/v0.1/torrents",
        &[("query", query)],
    )
    .unwrap();
    let results: Vec<Torrent> = client
        .request(Method::GET, url)
        .basic_auth("derp", Some("secret"))
        .send()?
        .json()?;
    Ok(results
        .into_iter()
        .map(|t| SearchResultItem {
            hits: 1,
            key: t.name,
        })
        .collect())
}
