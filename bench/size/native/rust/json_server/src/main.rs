// A JSON HTTP server: GET /items lists the items, POST /items adds one
// (JSON body {"name": ..., "qty": ...}), GET /items/{id} gets one.
use serde::{Deserialize, Serialize};
use std::sync::{Arc, RwLock};
use tiny_http::{Header, Method, Response, Server};

#[derive(Serialize, Clone)]
struct Item {
    id: usize,
    name: String,
    qty: i64,
}

#[derive(Deserialize)]
struct NewItem {
    name: String,
    #[serde(default = "one")]
    qty: i64,
}

fn one() -> i64 {
    1
}

fn json<T: Serialize>(code: u16, v: &T) -> Response<std::io::Cursor<Vec<u8>>> {
    let body = serde_json::to_vec(v).unwrap_or_default();
    Response::from_data(body)
        .with_status_code(code)
        .with_header(Header::from_bytes("Content-Type", "application/json").unwrap())
}

fn main() {
    let addr = std::env::var("ADDR").unwrap_or_else(|_| "127.0.0.1:8080".to_string());
    let server = Server::http(&addr).expect("listen");
    let items: Arc<RwLock<Vec<Item>>> = Arc::new(RwLock::new(Vec::new()));
    for mut req in server.incoming_requests() {
        let url = req.url().to_string();
        let resp = match (req.method(), url.as_str()) {
            (Method::Get, "/items") => json(200, &*items.read().unwrap()),
            (Method::Post, "/items") => {
                let mut body = String::new();
                let _ = req.as_reader().read_to_string(&mut body);
                match serde_json::from_str::<NewItem>(&body) {
                    Ok(n) => {
                        let mut l = items.write().unwrap();
                        let it = Item { id: l.len() + 1, name: n.name, qty: n.qty };
                        l.push(it.clone());
                        json(201, &it)
                    }
                    Err(e) => Response::from_string(e.to_string()).with_status_code(400),
                }
            }
            (Method::Get, p) if p.starts_with("/items/") => {
                match p["/items/".len()..].parse::<usize>() {
                    Ok(id) => match items.read().unwrap().iter().find(|i| i.id == id) {
                        Some(i) => json(200, i),
                        None => Response::from_string("not found").with_status_code(404),
                    },
                    Err(_) => Response::from_string("bad id").with_status_code(400),
                }
            }
            _ => Response::from_string("not found").with_status_code(404),
        };
        let _ = req.respond(resp);
    }
}
