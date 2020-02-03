use crate::server::{SearchQuery,Server};
use std::convert::Infallible;
use std::net::SocketAddr;
use std::sync::{Arc};
use warp::{Filter};

pub async fn search_handler(
    query: SearchQuery,
    server: Arc<Server>,
) -> Result<impl warp::Reply, Infallible> {
    let body = server.search_response(&query).await;
    Ok(warp::reply::json(&body))
}

pub async fn run_server(server: Arc<crate::server::Server>) {
    let route = warp::get()
        .and(warp::query::<SearchQuery>())
        .and(warp::any().map(move || server.clone()))
        .and_then(search_handler)
        .with(warp::cors().allow_any_origin());

    let addr = SocketAddr::from(([127, 0, 0, 1], 8080));
    warp::serve(route).run(addr).await;
}
