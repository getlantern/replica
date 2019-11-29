use url::form_urlencoded::parse;

pub fn get_terms_from_query_string<'a>(
    input: &'a [u8],
) -> impl Iterator<Item = std::borrow::Cow<'a, str>> {
    parse(input).filter_map(|(k, v)| if k == "term" { Some(v) } else { None })
}

use crate::IndexState;

pub fn search_response_body<I: AsRef<str>>(
    index: &IndexState,
    terms: impl Iterator<Item = I>,
) -> String {
    let mut keys = index.lock().unwrap().get_matches(terms);
    keys.push("".to_owned());
    keys.join("\n")
}
