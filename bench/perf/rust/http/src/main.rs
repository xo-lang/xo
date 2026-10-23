// Rust twin of bench/perf/http/go: an in-memory users CRUD API on axum
// (tokio multi-thread runtime) with serde_json, listening on loopback.
// Same routes, validation order, status codes and error messages as the Go
// server (handlers.go), written the way a Rust service is written.
use std::collections::{BTreeMap, HashMap};
use std::sync::RwLock;

use axum::body::Bytes;
use axum::extract::{DefaultBodyLimit, Path, State};
use axum::http::{header, StatusCode};
use axum::response::{IntoResponse, Response};
use axum::routing::get;
use axum::serve::ListenerExt;
use axum::Router;
use serde::Serialize;
use serde_json::{Map, Value};

const MAX_BODY_BYTES: usize = 1 << 20;

#[derive(Clone, Serialize)]
struct User {
    id: u64,
    name: String,
    email: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    nickname: Option<String>,
}

#[derive(Default)]
struct Store {
    next_id: u64,
    users: BTreeMap<u64, User>,
    by_email: HashMap<String, u64>,
}

enum StoreError {
    NotFound,
    DuplicateEmail,
}

impl StoreError {
    fn response(&self) -> Response {
        match self {
            StoreError::NotFound => error(StatusCode::NOT_FOUND, "user not found"),
            StoreError::DuplicateEmail => error(StatusCode::CONFLICT, "email already exists"),
        }
    }
}

struct Patch {
    name: Option<String>,
    email: Option<String>,
    // None: leave; Some(None): clear; Some(Some(n)): set
    nickname: Option<Option<String>>,
}

type Shared = std::sync::Arc<RwLock<Store>>;

fn json(status: StatusCode, body: Vec<u8>) -> Response {
    (status, [(header::CONTENT_TYPE, "application/json")], body).into_response()
}

fn error(status: StatusCode, msg: &str) -> Response {
    json(status, serde_json::to_vec(&serde_json::json!({ "error": msg })).unwrap())
}

fn user_json(status: StatusCode, u: &User) -> Response {
    json(status, serde_json::to_vec(u).unwrap())
}

fn validate_name(name: &str) -> Result<(), &'static str> {
    let n = name.chars().count();
    if !(1..=100).contains(&n) {
        return Err("name must be 1 to 100 characters");
    }
    Ok(())
}

fn validate_email(email: &str) -> Result<(), &'static str> {
    if !email.contains('@') {
        return Err("email must contain @");
    }
    Ok(())
}

// read_fields decodes a JSON object body, rejecting unknown keys.
fn read_fields(body: &Bytes) -> Result<Map<String, Value>, String> {
    let trimmed = body.trim_ascii();
    if trimmed.first() != Some(&b'{') {
        return Err("body must be a JSON object".into());
    }
    let fields: Map<String, Value> =
        serde_json::from_slice(trimmed).map_err(|_| "invalid JSON body".to_string())?;
    for k in fields.keys() {
        if !matches!(k.as_str(), "name" | "email" | "nickname") {
            return Err(format!("unknown field {:?}", k));
        }
    }
    Ok(fields)
}

// string_field returns the field if present; it must be a JSON string.
fn string_field(fields: &Map<String, Value>, key: &str) -> Result<Option<String>, String> {
    match fields.get(key) {
        None => Ok(None),
        Some(Value::String(s)) => Ok(Some(s.clone())),
        Some(_) => Err(format!("{key} must be a string")),
    }
}

// nickname_field: absent -> None, null -> Some(None), string -> Some(Some(s)).
fn nickname_field(fields: &Map<String, Value>) -> Result<Option<Option<String>>, String> {
    match fields.get("nickname") {
        None => Ok(None),
        Some(Value::Null) => Ok(Some(None)),
        Some(Value::String(s)) => Ok(Some(Some(s.clone()))),
        Some(_) => Err("nickname must be a string or null".into()),
    }
}

fn parse_id(id: &str) -> Option<u64> {
    id.parse::<u64>().ok().filter(|n| *n >= 1)
}

fn bad(msg: impl AsRef<str>) -> Response {
    error(StatusCode::BAD_REQUEST, msg.as_ref())
}

async fn create(State(s): State<Shared>, body: Bytes) -> Response {
    let fields = match read_fields(&body) {
        Ok(f) => f,
        Err(e) => return bad(e),
    };
    let name = match string_field(&fields, "name") {
        Err(e) => return bad(e),
        Ok(None) => return bad("name is required"),
        Ok(Some(n)) => n,
    };
    if let Err(e) = validate_name(&name) {
        return bad(e);
    }
    let email = match string_field(&fields, "email") {
        Err(e) => return bad(e),
        Ok(None) => return bad("email is required"),
        Ok(Some(n)) => n,
    };
    if let Err(e) = validate_email(&email) {
        return bad(e);
    }
    let nickname = match nickname_field(&fields) {
        Err(e) => return bad(e),
        Ok(n) => n.flatten(),
    };
    let mut st = s.write().unwrap();
    if st.by_email.contains_key(&email) {
        return StoreError::DuplicateEmail.response();
    }
    let id = st.next_id;
    st.next_id += 1;
    let u = User { id, name, email: email.clone(), nickname };
    st.users.insert(id, u.clone());
    st.by_email.insert(email, id);
    drop(st);
    user_json(StatusCode::CREATED, &u)
}

// list encodes under the read lock, as the Go and Xo servers do.
async fn list(State(s): State<Shared>) -> Response {
    let st = s.read().unwrap();
    let mut out = Vec::with_capacity(64 * st.users.len() + 3);
    serde_json::to_writer(&mut out, &st.users.values().collect::<Vec<_>>()).unwrap();
    drop(st);
    out.push(b'\n');
    json(StatusCode::OK, out)
}

async fn get_user(State(s): State<Shared>, Path(id): Path<String>) -> Response {
    let Some(id) = parse_id(&id) else { return StoreError::NotFound.response() };
    let st = s.read().unwrap();
    match st.users.get(&id) {
        Some(u) => user_json(StatusCode::OK, u),
        None => StoreError::NotFound.response(),
    }
}

async fn patch(State(s): State<Shared>, Path(id): Path<String>, body: Bytes) -> Response {
    let Some(id) = parse_id(&id) else { return StoreError::NotFound.response() };
    let fields = match read_fields(&body) {
        Ok(f) => f,
        Err(e) => return bad(e),
    };
    let mut p = Patch { name: None, email: None, nickname: None };
    match string_field(&fields, "name") {
        Err(e) => return bad(e),
        Ok(Some(n)) => {
            if let Err(e) = validate_name(&n) {
                return bad(e);
            }
            p.name = Some(n);
        }
        Ok(None) => {}
    }
    match string_field(&fields, "email") {
        Err(e) => return bad(e),
        Ok(Some(n)) => {
            if let Err(e) = validate_email(&n) {
                return bad(e);
            }
            p.email = Some(n);
        }
        Ok(None) => {}
    }
    match nickname_field(&fields) {
        Err(e) => return bad(e),
        Ok(n) => p.nickname = n,
    }
    let mut st = s.write().unwrap();
    let Some(cur) = st.users.get(&id) else { return StoreError::NotFound.response() };
    let old_email = cur.email.clone();
    if let Some(e) = &p.email {
        if *e != old_email && st.by_email.contains_key(e) {
            return StoreError::DuplicateEmail.response();
        }
    }
    let u = st.users.get_mut(&id).unwrap();
    if let Some(e) = p.email {
        u.email = e.clone();
        if e != old_email {
            st.by_email.remove(&old_email);
            st.by_email.insert(e, id);
        }
    }
    let u = st.users.get_mut(&id).unwrap();
    if let Some(n) = p.name {
        u.name = n;
    }
    if let Some(n) = p.nickname {
        u.nickname = n;
    }
    let u = u.clone();
    drop(st);
    user_json(StatusCode::OK, &u)
}

async fn delete(State(s): State<Shared>, Path(id): Path<String>) -> Response {
    let Some(id) = parse_id(&id) else { return StoreError::NotFound.response() };
    let mut st = s.write().unwrap();
    match st.users.remove(&id) {
        Some(u) => {
            st.by_email.remove(&u.email);
            StatusCode::NO_CONTENT.into_response()
        }
        None => StoreError::NotFound.response(),
    }
}

#[tokio::main]
async fn main() {
    let store: Shared = std::sync::Arc::new(RwLock::new(Store { next_id: 1, ..Default::default() }));
    let app = Router::new()
        .route("/users", get(list).post(create))
        .route("/users/{id}", get(get_user).patch(patch).delete(delete))
        .layer(DefaultBodyLimit::max(MAX_BODY_BYTES))
        .with_state(store);
    let port = std::env::var("PORT").unwrap_or_else(|_| "8080".into());
    let listener = tokio::net::TcpListener::bind(format!("127.0.0.1:{port}"))
        .await
        .unwrap()
        .tap_io(|tcp| {
            let _ = tcp.set_nodelay(true);
        });
    eprintln!("listening on 127.0.0.1:{port}");
    axum::serve(listener, app).await.unwrap();
}
