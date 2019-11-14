#![feature(proc_macro_hygiene, decl_macro)]
extern crate rocket;

use crate::s3::get_all_objects;
use crate::s3::receive_s3_events;
use crate::s3::tokenize_object_key;
use human_size::{Byte, Kilobyte};

use std::sync::{Arc, Mutex};

mod s3;
mod search;
mod server;

fn main() {
    let index = Arc::new(Mutex::new(search::Index::default()));
    let objects = get_all_objects();
    for obj in &objects {
        let key = obj.key.as_ref().unwrap();
        if let Err(err) = index.lock().unwrap().add_key(key) {
            eprintln!("error adding {:?} to index: {}", key, err)
        }
        println!(
            "{:>12} {} {:?}",
            // Only handles the precision flag, so we have to wrap it with another format.
            format!(
                "{:.1}",
                human_size::SpecificSize::new(obj.size.unwrap() as f64, Byte)
                    .unwrap()
                    .into::<Kilobyte>()
            ),
            obj.key.as_ref().unwrap(),
            tokenize_object_key(key)
        );
    }
    {
        let index = Arc::clone(&index);
        std::thread::spawn(move || receive_s3_events(&index));
    }
    server::run_server(index);
}

#[test]
fn test_human_byte_size_ignores_padding() {
    // When this fails, maybe human_size handles padding.
    assert_eq!(
        format!("{:5}", human_size::SpecificSize::new(1, Byte).unwrap()),
        "1 B"
    )
}
