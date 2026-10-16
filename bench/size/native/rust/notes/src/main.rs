// notes: examples/notes-service in Rust, for the binary size comparison
// (bench/results/binary-size-native-2026-10-16.md): users and expiring
// notes behind a bearer token check, tokens stored as SHA-256 digests and
// compared in constant time, Argon2id passwords and POST /login, a
// background purge, and two phase shutdown (the first signal stops
// accepting). The store is maps under a mutex (Rust's std has no SQL
// engine); HTTP is tiny_http, JSON serde_json, Argon2id RustCrypto's.
use argon2::password_hash::{PasswordHash, PasswordHasher, PasswordVerifier, SaltString};
use argon2::Argon2;
use chrono::{DateTime, Duration, Utc};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::collections::BTreeMap;
use std::sync::{Arc, Mutex};
use subtle::ConstantTimeEq;
use tiny_http::{Header, Method, Request, Response, Server};

struct User {
    id: i64,
    name: String,
    digest: String,
    password: Option<String>,
}

#[derive(Serialize, Clone)]
struct Note {
    id: i64,
    owner: i64,
    body: String,
    created: DateTime<Utc>,
    expires: DateTime<Utc>,
}

#[derive(Deserialize)]
struct NewNote {
    body: String,
    #[serde(default = "hour")]
    ttl_secs: i64,
}

fn hour() -> i64 {
    3600
}

#[derive(Deserialize)]
struct Credentials {
    name: String,
    password: String,
}

#[derive(Default)]
struct Store {
    users: Vec<User>,
    notes: BTreeMap<i64, Note>,
    next_id: i64,
}

fn digest(token: &str) -> String {
    Sha256::digest(token.as_bytes()).iter().map(|b| format!("{b:02x}")).collect()
}

fn random(n: usize) -> Vec<u8> {
    let mut b = vec![0u8; n];
    getrandom::fill(&mut b).expect("random");
    b
}

fn new_token() -> String {
    const A: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";
    random(32).iter().map(|b| A[(*b & 63) as usize] as char).collect()
}

fn hash_password(pw: &str) -> String {
    let salt = SaltString::encode_b64(&random(16)).expect("salt");
    Argon2::default().hash_password(pw.as_bytes(), &salt).expect("hash").to_string()
}

fn verify_password(pw: &str, phc: &str) -> bool {
    match PasswordHash::new(phc) {
        Ok(h) => Argon2::default().verify_password(pw.as_bytes(), &h).is_ok(),
        Err(_) => false,
    }
}

type Resp = Response<std::io::Cursor<Vec<u8>>>;

fn json<T: Serialize>(code: u16, v: &T) -> Resp {
    Response::from_data(serde_json::to_vec(v).unwrap_or_default())
        .with_status_code(code)
        .with_header(Header::from_bytes("Content-Type", "application/json").unwrap())
}

fn fail(code: u16, msg: &str) -> Resp {
    json(code, &serde_json::json!({ "error": msg }))
}

fn body(req: &mut Request) -> String {
    let mut s = String::new();
    let _ = req.as_reader().read_to_string(&mut s);
    s
}

/// The user of the request's bearer token, compared in constant time.
fn auth(store: &Store, req: &Request) -> Option<i64> {
    let h = req.headers().iter().find(|h| h.field.equiv("Authorization"))?;
    let t = h.value.as_str().strip_prefix("Bearer ")?.trim();
    if t.is_empty() {
        return None;
    }
    let d = digest(t);
    store.users.iter().find(|u| bool::from(u.digest.as_bytes().ct_eq(d.as_bytes()))).map(|u| u.id)
}

fn handle(store: &Mutex<Store>, req: &mut Request) -> Resp {
    let url = req.url().to_string();
    let method = req.method().clone();
    match (method, url.as_str()) {
        (Method::Get, "/health") => json(200, &serde_json::json!({ "status": "ok" })),
        (Method::Post, "/login") => {
            let c: Credentials = match serde_json::from_str(&body(req)) {
                Ok(c) => c,
                Err(e) => return fail(400, &e.to_string()),
            };
            let phc = store.lock().unwrap().users.iter().find(|u| u.name == c.name).and_then(|u| u.password.clone());
            match phc {
                Some(p) if verify_password(&c.password, &p) => {
                    let t = new_token();
                    let mut s = store.lock().unwrap();
                    if let Some(u) = s.users.iter_mut().find(|u| u.name == c.name) {
                        u.digest = digest(&t);
                    }
                    json(200, &serde_json::json!({ "token": t }))
                }
                _ => fail(401, "missing or invalid bearer token"),
            }
        }
        (m, p) if p == "/notes" || p.starts_with("/notes/") => {
            let uid = match auth(&store.lock().unwrap(), req) {
                Some(u) => u,
                None => return fail(401, "missing or invalid bearer token"),
            };
            if p == "/notes" {
                if m == Method::Post {
                    let n: NewNote = match serde_json::from_str(&body(req)) {
                        Ok(n) => n,
                        Err(e) => return fail(400, &e.to_string()),
                    };
                    if n.body.is_empty() || n.ttl_secs <= 0 {
                        return fail(400, "body must not be empty and ttl_secs positive");
                    }
                    let now = Utc::now();
                    let mut s = store.lock().unwrap();
                    s.next_id += 1;
                    let nt = Note { id: s.next_id, owner: uid, body: n.body, created: now, expires: now + Duration::seconds(n.ttl_secs) };
                    s.notes.insert(nt.id, nt.clone());
                    return json(201, &nt);
                }
                let s = store.lock().unwrap();
                let out: Vec<&Note> = s.notes.values().filter(|n| n.owner == uid).collect();
                return json(200, &out);
            }
            let id: i64 = match p["/notes/".len()..].parse() {
                Ok(i) => i,
                Err(_) => return fail(400, "bad id"),
            };
            let mut s = store.lock().unwrap();
            match s.notes.get(&id).filter(|n| n.owner == uid).cloned() {
                None => fail(404, &format!("note {id} not found")),
                Some(n) if m == Method::Delete => {
                    s.notes.remove(&n.id);
                    Response::from_data(Vec::new()).with_status_code(204)
                }
                Some(n) => json(200, &n),
            }
        }
        _ => fail(404, "not found"),
    }
}

fn main() {
    let store = Arc::new(Mutex::new(Store::default()));
    let token = std::env::var("NOTES_TOKEN").unwrap_or_else(|_| {
        let t = new_token();
        println!("admin token: {t}");
        t
    });
    let password = std::env::var("NOTES_PASSWORD").ok().map(|p| hash_password(&p));
    store.lock().unwrap().users.push(User { id: 1, name: "admin".into(), digest: digest(&token), password });
    let addr = std::env::var("NOTES_ADDR").unwrap_or_else(|_| "127.0.0.1:8080".to_string());
    let server = Arc::new(Server::http(&addr).expect("listen"));
    eprintln!("listening addr={addr}");
    // the first signal stops accepting; requests in flight finish
    let stop = server.clone();
    ctrlc::set_handler(move || stop.unblock()).expect("signal handler");
    let purger = store.clone();
    std::thread::spawn(move || loop {
        std::thread::sleep(std::time::Duration::from_secs(60));
        let now = Utc::now();
        let mut s = purger.lock().unwrap();
        let before = s.notes.len();
        s.notes.retain(|_, n| n.expires > now);
        let n = before - s.notes.len();
        if n > 0 {
            eprintln!("purged expired notes count={n}");
        }
    });
    for mut req in server.incoming_requests() {
        let resp = handle(&store, &mut req);
        let _ = req.respond(resp);
    }
    eprintln!("stopped");
}
