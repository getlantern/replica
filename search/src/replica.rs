use serde::{Serialize, Serializer};

pub struct Link {
    pub info_hash: Option<String>,
    pub display_name: Option<String>,
}

impl ToString for Link {
    fn to_string(&self) -> String {
        let query_str = match &self.info_hash {
            // Every implementation wants to escape ':', but we don't.
            Some(ih) => format!("xt=urn:btih:{}", ih),
            None => "".to_string(),
        };
        let mut query = url::form_urlencoded::Serializer::new(query_str);
        if let Some(dn) = &self.display_name {
            query.append_pair("dn", &dn);
        }
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
            info_hash: Some(ih.to_owned()),
            display_name: Some(dn.to_owned()),
        };
        assert_eq!(l.to_string(), "magnet:?xt=urn:btih:abcd&dn=yo");
    }
    #[test]
    fn test_no_infohash() {
        let l = Link {
            info_hash: None,
            display_name: Some("hello there!".to_string()),
        };
        assert_eq!(l.to_string(), "magnet:?dn=hello+there%21");
    }
}
