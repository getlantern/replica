mod magnetico;

pub use magnetico::*;

pub struct SearchResultItem {
    pub torrent_name: String,
    pub info_hash: String,
    pub file_path: String,
    pub size: FileSize,
}

pub type FileSize = i64;

impl SearchResultItem {
    pub fn score<'a, I>(&self, terms: I) -> usize
    where
        I: Iterator<Item = &'a str>,
    {
        let tokens: Vec<&str> = [&self.torrent_name, &self.file_path]
            .iter()
            .map(|s| s.rsplitn(2, '.'))
            .flatten()
            .map(str::split_whitespace)
            .flatten()
            .collect();
        let mut ok = 0;
        for t in terms {
            if tokens.contains(&t) {
                ok += 1;
            }
        }
        ok
    }
}
