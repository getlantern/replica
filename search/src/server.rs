use crate::Index;
use rocket::{get, routes};

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
pub fn search(rest: SearchQuery, index: rocket::State<Index>) -> rocket::response::Content<String> {
    for t in rest.0 {
        if let Some(keys) = index.terms.get(&t) {
            for k in keys {
                println!("{}", k)
            }
        }
    }
    rocket::response::Content(
        rocket::http::ContentType::new("text", "uri-list"),
        "".to_owned(),
    )
}

pub fn run_server(index: Index) {
    rocket::ignite()
        .mount("/", routes![search])
        .manage(index)
        .launch();
}
