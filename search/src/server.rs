use crate::search::Index;
use rocket::{get, routes, State};
use std::sync::{Arc, Mutex};

pub struct SearchQuery(Vec<String>);

impl<'a> rocket::request::FromQuery<'a> for SearchQuery {
    type Error = ();
    fn from_query(q: rocket::request::Query) -> Result<Self, Self::Error> {
        Ok(SearchQuery(
            q.into_iter()
                .map(|fi| fi.key_value_decoded())
                .filter_map(|(k, v)| if k == "term" { Some(v) } else { None })
                .collect(),
        ))
    }
}

#[get("/?<rest..>")]
pub fn search(rest: SearchQuery, index: State<Arc<Mutex<Index>>>) -> rocket::response::Content<String> {
    let mut keys = index.lock().unwrap().get_matches(rest.0.iter());
    keys.push("".to_owned());
    let body = keys.join("\n");
    rocket::response::Content(rocket::http::ContentType::new("text", "uri-list"), body)
}

pub fn run_server(index: Arc<Mutex<Index>>) {
    rocket::ignite()
        .mount("/", routes![search])
        .manage(index)
        .launch();
}
