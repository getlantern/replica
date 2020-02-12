#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let key = std::env::args()
        .skip(1)
        .collect::<Vec<_>>()
        .first()
        .unwrap()
        .to_string();
    println!("{}", key);
    println!("{:?}", search::s3::get_infohash(key).await?);
    Ok(())
}
