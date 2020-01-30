mod magnetico;

pub use magnetico::*;

pub struct SearchResultItem {
    pub torrent_name: String,
    pub info_hash: String,
    pub file_path: String,
    pub size: FileSize,
}

pub type FileSize = i64;
