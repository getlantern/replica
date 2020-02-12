use super::types::*;
use serde::{Serialize, Serializer};

pub const TRACKERS: &[&str] = &["http://s3-tracker.ap-southeast-1.amazonaws.com:6969/announce"];

pub struct Link {
    pub info_hash: String,
    pub display_name: Option<String>,
    pub trackers: Vec<String>,
    pub acceptable_source: Option<String>,
    pub exact_source: Option<String>,
}

impl ToString for Link {
    fn to_string(&self) -> String {
        let query_str = format!("xt=urn:btih:{}", self.info_hash);
        let mut query = url::form_urlencoded::Serializer::new(query_str);
        let mut append_some = |key, opt: &Option<String>| {
            if let Some(val) = opt.as_ref() {
                query.append_pair(key, val);
            }
        };
        append_some("dn", &self.display_name);
        append_some("xs", &self.exact_source);
        append_some("as", &self.acceptable_source);
        query.extend_pairs(self.trackers.iter().map(|v| ("tr", v)));
        format!("magnet:?{}", query.finish())
    }
}

impl Serialize for Link {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        serializer.serialize_str(&self.to_string())
    }
}

#[cfg(test)]
mod test {
    use super::*;
    #[test]
    fn test_link_to_string() {
        let ih = "abcd";
        let dn = "yo";
        let l = Link {
            info_hash: ih.to_owned(),
            display_name: Some(dn.to_owned()),
            trackers: vec!["a".to_string(), "b".to_string()],
            exact_source: None,
            acceptable_source: None,
        };
        assert_eq!(l.to_string(), "magnet:?xt=urn:btih:abcd&dn=yo&tr=a&tr=b");
    }
}
