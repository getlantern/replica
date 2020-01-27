use crate::search::Index;
use crate::server::SearchQuery;
use std::convert::Infallible;
use std::net::{SocketAddr};
use std::sync::{Arc, Mutex};
use warp::{self, Filter};

type IndexState = Arc<Mutex<Index>>;

use crate::server::search_response;

pub async fn search_handler(
    query: SearchQuery,
    index: IndexState,
) -> Result<impl warp::Reply, Infallible> {
    let body = search_response(&index, query).await;
    Ok(warp::reply::json(&body))
}

fn with_index(
    index: Arc<Mutex<Index>>,
) -> impl Filter<Extract = (IndexState,), Error = std::convert::Infallible> + Clone {
    warp::any().map(move || index.clone())
}

pub async fn run_server(index: IndexState) {
    let route = warp::get()
        .and(warp::query::<SearchQuery>())
        .and(with_index(index))
        .and_then(search_handler);

    let addr = SocketAddr::from(([127, 0, 0, 1], 8080));
    warp::serve(route).run(addr).await;
}
