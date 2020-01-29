use serde::Deserialize;

#[derive(Deserialize)]
pub struct Torrent {
    #[serde(rename = "infoHash")]
    pub info_hash: String,
    pub id: u64,
    pub name: String,
    pub size: u64,
    #[serde(rename = "discoveredOn")]
    pub discovered_on: u64,
    #[serde(rename = "nFiles")]
    pub n_files: u64,
    pub relevance: f64,
}
