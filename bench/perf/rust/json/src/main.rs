// Rust twin of bench/perf/jsontwin: serde_json decode and encode of 1000
// users, 100 rounds each. Usage: jsontwin decode|encode.
use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize)]
struct User {
    id: i64,
    name: String,
    email: String,
    score: f64,
    active: bool,
    tags: Vec<String>,
}

const DOCS: usize = 1000;
const ROUNDS: usize = 100;

fn users() -> Vec<User> {
    let mut us = Vec::with_capacity(DOCS);
    let mut seed: i64 = 12345;
    for _ in 0..DOCS {
        seed = (seed * 1103515245 + 12345) % 2147483648;
        let n = seed % 100000;
        us.push(User {
            id: n,
            name: format!("user {n}"),
            email: format!("user_{n}@example.com"),
            score: (seed % 1000) as f64 + 0.5,
            active: seed % 2 == 0,
            tags: (0..seed % 5).map(|t| format!("t{t}")).collect(),
        });
    }
    us
}

// The document built by hand, not with the encoder under test.
fn text(u: &User) -> String {
    let tags: Vec<String> = u.tags.iter().map(|t| format!("\"{t}\"")).collect();
    format!(
        "{{\"id\":{},\"name\":\"{}\",\"email\":\"{}\",\"score\":{:.1},\"active\":{},\"tags\":[{}]}}",
        u.id,
        u.name,
        u.email,
        u.score,
        u.active,
        tags.join(",")
    )
}

fn main() {
    let w = std::env::args().nth(1).unwrap_or_else(|| "decode".into());
    let us = users();
    let mut h: i64 = 0;
    match w.as_str() {
        "decode" => {
            let ds: Vec<String> = us.iter().map(text).collect();
            for _ in 0..ROUNDS {
                for d in &ds {
                    let u: User = serde_json::from_str(d).unwrap();
                    h = (h * 31
                        + u.id
                        + u.name.len() as i64
                        + u.email.len() as i64
                        + u.tags.len() as i64
                        + (u.score as i64) % 100
                        + u.active as i64)
                        % 1000000007;
                }
            }
        }
        "encode" => {
            for _ in 0..ROUNDS {
                for u in &us {
                    let b = serde_json::to_vec(u).unwrap();
                    h = (h * 31 + b.len() as i64) % 1000000007;
                }
            }
        }
        _ => {
            eprintln!("unknown workload {w}");
            std::process::exit(2);
        }
    }
    println!("{w} {h}");
}
