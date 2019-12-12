use url::form_urlencoded::parse;
use serde::{Serialize};

pub fn get_terms_from_query_string(
    input: &[u8],
) -> impl Iterator<Item = std::borrow::Cow<'_, str>> {
    parse(input).filter_map(|(k, v)| if k == "term" { Some(v) } else { None })
}

use crate::IndexState;

#[derive(Serialize)]
pub struct SearchResultItem {
	key: String
}

type SearchResult = Vec<SearchResultItem>;

pub fn search_response<I: AsRef<str>>(
    index: &IndexState,
    terms: impl Iterator<Item = I>,
) -> SearchResult {
    index.lock().unwrap().get_matches(terms).into_iter().map(|key| SearchResultItem{key}).collect()
}

#[deprecated]
pub use search_response as search_response_body;