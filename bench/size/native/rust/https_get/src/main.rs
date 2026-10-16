// GET one URL over HTTPS (URL, default https://example.com/) and print the
// status and the body's length (ureq over rustls).
fn main() {
    let url = std::env::var("URL").unwrap_or_else(|_| "https://example.com/".to_string());
    let mut r = match ureq::get(&url).call() {
        Ok(r) => r,
        Err(e) => {
            eprintln!("{e}");
            std::process::exit(1);
        }
    };
    let status = r.status().as_u16();
    let body = r.body_mut().read_to_vec().unwrap_or_default();
    println!("{status} {}", body.len());
}
