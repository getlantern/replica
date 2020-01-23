use crate::search::Index;
use crate::server::SearchQuery;
use actix_web::web::Query;
use actix_web::{web, App, HttpServer};
use std::sync::{Arc, Mutex};

type IndexState = Arc<Mutex<Index>>;

use crate::server::search_response;

use actix_web::HttpResponse;

fn search_handler(s: Query<SearchQuery>, index: web::Data<IndexState>) -> actix_web::HttpResponse {
    let body = search_response(&index, s.into_inner());
    HttpResponse::Ok().json(body)
}

pub async fn run_server(index: Arc<Mutex<Index>>) {
    HttpServer::new(move || {
        let index = Arc::clone(&index);
        App::new()
            .data(index)
            .route("/", web::get().to(search_handler))
    })
    .bind("localhost:8000")
    .unwrap()
    .run()
    .unwrap();
}
