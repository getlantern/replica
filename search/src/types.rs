use serde::Serialize;

use chrono::offset::TimeZone;

type WrappedDateTime = chrono::DateTime<chrono::Utc>;

pub type FileSize = i64;

#[derive(Debug, Serialize, Copy, Clone, PartialEq)]
pub struct DateTime(WrappedDateTime);

pub use anyhow::Result;

// #[derive(Debug)]
pub struct InfoHash(bip_metainfo::InfoHash);

impl From<bip_metainfo::InfoHash> for InfoHash {
    fn from(f: bip_metainfo::InfoHash) -> Self {
        Self(f)
    }
}

use std::fmt;

impl fmt::Debug for InfoHash {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", hex::encode(self.0))
    }
}

// pub use bip_metainfo::InfoHash;

impl DateTime {
    pub fn now() -> Self {
        Self(chrono::Utc::now())
    }
    pub fn parse_from_s3(s: &str) -> Result<Self> {
        chrono::DateTime::parse_from_rfc3339(s)
            .map(|x| Self(WrappedDateTime::from(x)))
            .map_err(anyhow::Error::new)
    }
}

use crate::bittorrent::Epoch;

impl From<Epoch> for DateTime {
    fn from(t: Epoch) -> Self {
        Self(chrono::Utc.timestamp(t.0, 0))
    }
}

#[derive(Eq, Hash, PartialEq)]
pub struct NormalizedToken(String);

impl NormalizedToken {
    pub fn new(s: &str) -> Self {
        Self(s.to_lowercase())
    }
}

pub type Tokenizer = &'static (dyn Fn(&str) -> Result<Vec<String>> + Send + Sync);

pub type TokenNormalizer = fn(&str) -> NormalizedToken;

#[cfg(test)]
mod test {
    use super::*;
    use log::*;

    #[test]
    fn test_parse_from_s3() -> Result<()> {
        let parsed = DateTime::parse_from_s3("2020-01-30T10:32:16.123Z")?;
        info!("{:?}", parsed);
        assert_ne!(parsed, DateTime::now());
        Ok(())
    }
}

pub use crate::replica::Link as ReplicaLink;
