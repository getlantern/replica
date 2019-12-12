use crate::search::Index;
use actix_web::{web, App, HttpRequest, HttpServer};
use std::sync::{Arc, Mutex};

type IndexState = Arc<Mutex<Index>>;

use crate::server::search_response;

use actix_web::HttpResponse;

fn search_handler(req: HttpRequest, index: web::Data<IndexState>) -> actix_web::HttpResponse {
    let terms = crate::server::get_terms_from_query_string(req.query_string().as_bytes());
    let body = search_response(&index, terms);
    HttpResponse::Ok()
        .json(body)
}

pub fn run_server(index: Arc<Mutex<Index>>) {
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
