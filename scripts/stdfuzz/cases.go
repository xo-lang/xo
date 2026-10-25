package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"net"
	"net/url"
	"strings"
	"time"
)

// jsonHelpers, urlHelpers, baseHelpers and timeHelpers wrap the match, whose
// arms Xo wants on separate lines, so each case stays one line of source.
const jsonHelpers = `
fn fz_json(text: Str) -> Str {
  match json.parse_value(text) {
    Ok(v) => fz_hx(v.encode())
    Err(e) => "err:${e.display()}"
  }
}
`

const urlHelpers = `
fn fz_un_q(s: Str) -> Str {
  match url.query_unescape(s) {
    Ok(v) => fz_hx(v)
    Err(e) => "err:${e.display()}"
  }
}
fn fz_un_p(s: Str) -> Str {
  match url.path_unescape(s) {
    Ok(v) => fz_hx(v)
    Err(e) => "err:${e.display()}"
  }
}
fn fz_query(s: Str) -> Str {
  match url.parse_query(s) {
    Ok(v) => fz_hx(v.encode())
    Err(e) => "err:${e.display()}"
  }
}
`

const baseHelpers = `
fn fz_b64(s: Str, enc: base64.Base64) -> Str {
  match base64.decode(s, enc) {
    Ok(v) => v.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
fn fz_b32(s: Str) -> Str {
  match base32.decode(s, base32.Base32.Std) {
    Ok(v) => v.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
`

const timeHelpers = `
fn fz_dur(s: Str) -> Str {
  match Duration.parse(s) {
    Ok(d) => fz_hx("${d}")
    Err(e) => "err:${e.display()}"
  }
}
fn fz_stamp(s: Str) -> Str {
  match Time.parse_rfc3339(s) {
    Some(t) => fz_hx(t.format_rfc3339())
    None => "none"
  }
}
`

const regexpHelpers = `
fn fz_re_m(p: Str, s: Str) -> Str {
  match regexp.compile(p) {
    Ok(r) => fz_bool(r.is_match(s))
    Err(e) => "err:${e.display()}"
  }
}
fn fz_re_f(p: Str, s: Str) -> Str {
  match regexp.compile(p) {
    Ok(r) => fz_opt_s(r.find(s))
    Err(e) => "err:${e.display()}"
  }
}
fn fz_re_a(p: Str, s: Str) -> Str {
  match regexp.compile(p) {
    Ok(r) => fz_ls(r.find_all(s))
    Err(e) => "err:${e.display()}"
  }
}
fn fz_re_sp(p: Str, s: Str) -> Str {
  match regexp.compile(p) {
    Ok(r) => fz_ls(r.split(s))
    Err(e) => "err:${e.display()}"
  }
}
fn fz_re_r(p: Str, s: Str, repl: Str) -> Str {
  match regexp.compile(p) {
    Ok(r) => fz_hx(r.replace_all(s, repl))
    Err(e) => "err:${e.display()}"
  }
}
`

const csvHelpers = `
fn fz_csv(s: Str) -> Str {
  match csv.parse(s) {
    Ok(rows) => fz_lss(rows)
    Err(e) => "err:${e.display()}"
  }
}
fn fz_csv_semi(s: Str) -> Str {
  match csv.parse(s, sep: ';') {
    Ok(rows) => fz_lss(rows)
    Err(e) => "err:${e.display()}"
  }
}
fn fz_csv_lazy(s: Str) -> Str {
  match csv.parse(s, lazy_quotes: true, trim_leading_space: true) {
    Ok(rows) => fz_lss(rows)
    Err(e) => "err:${e.display()}"
  }
}
`

const netipHelpers = `
fn fz_addr(s: Str) -> Str {
  match netip.parse_addr(s) {
    Ok(a) => fz_hx("${a}")
    Err(e) => "err:${e.display()}"
  }
}
fn fz_addr6(s: Str) -> Str {
  match netip.parse_addr(s) {
    Ok(a) => fz_hx("${fz_bools(a.is_4(), a.is_6(), a.is_4in6(), a.is_loopback(), a.is_private())}-${a.bytes().to_hex()}")
    Err(e) => "err:${e.display()}"
  }
}
fn fz_pfx(s: Str) -> Str {
  match netip.parse_prefix(s) {
    Ok(p) => fz_hx("${p} ${p.masked()}")
    Err(e) => "err:${e.display()}"
  }
}
`

const bigHelpers = `
fn fz_big_ops(a: Int, b: Int) -> Str {
  let x = big.from_int(a)
  let y = big.from_int(b)
  let q = if y.sign() == 0 { "-" } else { "${x.quo(y)}/${x.rem(y)}" }
  fz_hx("${x.add(y)};${x.sub(y)};${x.mul(y)};${x.cmp(y)};${x.cmp_abs(y)};${q};${x.and(y)};${x.or(y)};${x.xor(y)};${x.gcd(y)};${x.lsh(3)};${x.rsh(3)}")
}
fn fz_big_parse(s: Str) -> Str {
  match big.parse(s) {
    Ok(x) => fz_hx("${x}")
    Err(e) => "err:${e.display()}"
  }
}
fn fz_big_hex(s: Str) -> Str {
  match big.parse(s, base: 16) {
    Ok(x) => fz_hx("${x}")
    Err(e) => "err:${e.display()}"
  }
}
`

const compressHelpers = `
fn fz_gz(b: Bytes, level: compress.Level) -> Str { fz_hx(compress.gzip(b, level).to_hex()) }
fn fz_gz_rt(b: Bytes, level: compress.Level) -> Str {
  match compress.gunzip(compress.gzip(b, level)) {
    Ok(x) => x.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
fn fz_deflate_rt(b: Bytes) -> Str {
  match compress.inflate(compress.deflate(b)) {
    Ok(x) => x.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
fn fz_zlib_rt(b: Bytes) -> Str {
  match compress.unzlib(compress.zlib(b)) {
    Ok(x) => x.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
fn fz_gunzip(b: Bytes) -> Str {
  match compress.gunzip(b) {
    Ok(x) => x.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
fn fz_inflate(b: Bytes) -> Str {
  match compress.inflate(b) {
    Ok(x) => x.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
fn fz_unzlib(b: Bytes) -> Str {
  match compress.unzlib(b) {
    Ok(x) => x.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
fn fz_stream(b: Bytes, f: compress.Format) -> Str {
  var c = compress.compressor(f)
  let w = c.write(b)
  let fl = c.flush()
  let fin = c.finish()
  fz_hx("${w.to_hex()};${fl.to_hex()};${fin.to_hex()}")
}
`

const archiveHelpers = `
fn fz_tar(name: Str, b: Bytes) -> Str {
  match archive.tar([archive.file(name, b)]) {
    Ok(t) => t.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
fn fz_zip(name: Str, b: Bytes) -> Str {
  match archive.zip([archive.file(name, b)]) {
    Ok(t) => t.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
fn fz_entries(es: List[archive.Entry]) -> Str {
  var out = ""
  for e in es {
    if out != "" { out = out + ";" }
    out = out + fz_hx("${e.path}:${e.data.to_hex()}:${e.mode}:${e.is_dir}:${e.modified.unix()}")
  }
  "${es.len()}/${out}"
}
fn fz_untar(name: Str, b: Bytes) -> Str {
  match archive.tar([archive.file(name, b)]) {
    Ok(t) => match archive.untar(t) {
      Ok(es) => fz_entries(es)
      Err(e) => "err:${e.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
fn fz_unzip(name: Str, b: Bytes) -> Str {
  match archive.zip([archive.file(name, b)]) {
    Ok(t) => match archive.unzip(t) {
      Ok(es) => fz_entries(es)
      Err(e) => "err:${e.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
fn fz_unzip_raw(b: Bytes) -> Str {
  match archive.unzip(b) {
    Ok(es) => fz_entries(es)
    Err(e) => "err:${e.display()}"
  }
}
fn fz_untar_raw(b: Bytes) -> Str {
  match archive.untar(b) {
    Ok(es) => fz_entries(es)
    Err(e) => "err:${e.display()}"
  }
}
fn fz_agunzip(b: Bytes) -> Str {
  match archive.gunzip(b) {
    Ok(x) => x.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
`

const mimeHelpers = `
fn fz_mt(v: Str) -> Str {
  match mime.parse_media_type(v) {
    Ok(m) => fz_hx(mime.format_media_type(m.kind, m.params))
    Err(e) => "err:${e.display()}"
  }
}
fn fz_mime_ext(ext: Str) -> Str { fz_opt_s(mime.type_by_extension(ext)) }
fn fz_mime_exts(kind: Str) -> Str {
  match mime.extensions_by_type(kind) {
    Ok(xs) => fz_ls(xs)
    Err(e) => "err:${e.display()}"
  }
}
`

const multipartHelpers = `
fn fz_parts(ps: List[multipart.Part]) -> Str {
  var out = ""
  for p in ps {
    if out != "" { out = out + ";" }
    out = out + fz_hx("${p.name}:${p.filename ?? ""}:${p.data.to_hex()}")
  }
  "${ps.len()}/${out}"
}
fn fz_mp(body: Bytes, boundary: Str) -> Str {
  match multipart.parse(body, boundary) {
    Ok(ps) => fz_parts(ps)
    Err(e) => "err:${e.display()}"
  }
}
fn fz_mp_write(name: Str, b: Bytes, boundary: Str) -> Str {
  let part = multipart.Part{name: name, filename: None, headers: {}, data: b}
  fz_hx(multipart.write([part], boundary).to_hex())
}
fn fz_mp_form(body: Bytes, boundary: Str) -> Str {
  match multipart.parse_form(body, boundary) {
    Ok(f) => fz_hx("${f.values.encode()}")
    Err(e) => "err:${e.display()}"
  }
}
`

const xmlHelpers = `
fn fz_xml(text: Str) -> Str {
  match xml.parse(text) {
    Ok(e) => match e.encode() {
      Ok(s) => fz_hx(s)
      Err(err) => "err:${err.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
fn fz_xml_loose(text: Str) -> Str {
  match xml.parse(text, strict: false) {
    Ok(e) => match e.encode() {
      Ok(s) => fz_hx(s)
      Err(err) => "err:${err.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
fn fz_xml_n(text: Str) -> Str {
  match xml.tokens(text) {
    Ok(ts) => "${ts.len()}"
    Err(e) => "err:${e.display()}"
  }
}
`

const sqlHelpers = `
fn fz_sq_i(r: sql.Row, c: Str) -> Str {
  match r.int(c) {
    Ok(v) => "${v}"
    Err(e) => "!${e.display()}"
  }
}
fn fz_sq_s(r: sql.Row, c: Str) -> Str {
  match r.str(c) {
    Ok(v) => v
    Err(e) => "!${e.display()}"
  }
}
fn fz_sq_f(r: sql.Row, c: Str) -> Str {
  match r.float(c) {
    Ok(v) => "${v}"
    Err(e) => "!${e.display()}"
  }
}
fn fz_sq_by(r: sql.Row, c: Str) -> Str {
  match r.bytes(c) {
    Ok(v) => v.to_hex()
    Err(e) => "!${e.display()}"
  }
}
fn fz_sq_io(r: sql.Row, c: Str) -> Str {
  match r.int_opt(c) {
    Ok(v) => match v {
      Some(x) => "${x}"
      None => "null"
    }
    Err(e) => "!${e.display()}"
  }
}
fn fz_sq_bo(r: sql.Row, c: Str) -> Str {
  match r.bytes_opt(c) {
    Ok(v) => match v {
      Some(x) => x.to_hex()
      None => "null"
    }
    Err(e) => "!${e.display()}"
  }
}
fn fz_row(r: sql.Row) -> Str {
  "${fz_sq_i(r, "id")}|${fz_sq_s(r, "name")}|${fz_sq_io(r, "n")}|${fz_sq_f(r, "f")}|${fz_sq_by(r, "b")}|${fz_sq_bo(r, "bnull")}"
}
fn fz_rows(rs: List[sql.Row]) -> Str {
  var out = ""
  for r in rs {
    if out != "" { out = out + ";" }
    out = out + fz_row(r)
  }
  "${rs.len()}/${out}"
}
fn fz_sql_rt(s: Str, n: Int, b: Bytes) -> Str {
  let schema = "create table t (id integer primary key, name text, n integer, f real, b blob, bnull blob)"
  match sql.memory(schema) {
    Ok(db) => match db.exec("insert into t (id, name, n, f, b) values ($1, $2, $3, $4, $5)", [sql.int(7), sql.str(s), sql.int(n), sql.float(1.25), sql.bytes(b)]) {
      Ok(res) => match db.query("select id, name, n, f, b, bnull from t order by id") {
        Ok(rows) => fz_hx("${fz_rows(rows)};${res.rows_affected};${res.last_insert_id}")
        Err(e) => "err:${e.display()}"
      }
      Err(e) => "err:${e.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
fn fz_sql_norows() -> Str {
  match sql.memory("create table t (id integer primary key)") {
    Ok(db) => match db.query_one("select id from t where id = 1") {
      Ok(r) => "unexpected row ${fz_sq_i(r, "id")}"
      Err(e) => "err:${e.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
fn fz_sql_mismatch(s: Str) -> Str {
  match sql.memory("create table t (name text)") {
    Ok(db) => match db.exec("insert into t (name) values ($1)", [sql.str(s)]) {
      Ok(_) => match db.query_one("select name from t") {
        Ok(r) => fz_hx("int:${fz_sq_i(r, "name")};nocol:${fz_sq_s(r, "nope")}")
        Err(e) => "err:${e.display()}"
      }
      Err(e) => "err:${e.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
fn fz_sql_bad(q: Str) -> Str {
  match sql.memory("") {
    Ok(db) => match db.query(q) {
      Ok(rows) => "rows ${rows.len()}"
      Err(e) => "err:${e.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
fn fz_sql_dup(n: Int) -> Str {
  match sql.memory("create table t (id integer primary key, v text)") {
    Ok(db) => match db.exec("insert into t (id, v) values ($1, $2)", [sql.int(n), sql.str("a")]) {
      Ok(_) => match db.exec("insert into t (id, v) values ($1, $2)", [sql.int(n), sql.str("b")]) {
        Ok(_) => "unexpected second insert"
        Err(e) => "err:${e.display()}"
      }
      Err(e) => "err:${e.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
fn fz_sql_tx(n: Int) -> Str {
  match sql.memory("create table t (id integer primary key)") {
    Ok(db) => match db.begin() {
      Ok(tx) => match tx.exec("insert into t (id) values ($1)", [sql.int(n)]) {
        Ok(_) => match tx.rollback() {
          Ok(_) => match db.query("select id from t") {
            Ok(rows) => "rolled back ${rows.len()}"
            Err(e) => "err:${e.display()}"
          }
          Err(e) => "err:${e.display()}"
        }
        Err(e) => "err:${e.display()}"
      }
      Err(e) => "err:${e.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
`

const httpHelpers = `
fn fz_cookies(cs: List[http.Cookie]) -> Str {
  var out = ""
  for c in cs {
    if out != "" { out = out + ";" }
    out = out + fz_hx(c.display())
  }
  "${cs.len()}/${out}"
}
fn fz_cookie(line: Str) -> Str {
  match http.parse_cookie(line) {
    Ok(cs) => fz_cookies(cs)
    Err(e) => "err:${e.display()}"
  }
}
fn fz_set_cookie(line: Str) -> Str {
  match http.parse_set_cookie(line) {
    Ok(c) => fz_hx(c.display())
    Err(e) => "err:${e.display()}"
  }
}
fn fz_req(url: Str) -> Str {
  let r = http.new_request("GET", url)
  fz_hx("${fz_opt_s(r.query("a"))}|${fz_ls(r.query_all("a"))}|${fz_opt_s(r.header("X"))}|${fz_ls(r.header_values("x"))}")
}
fn fz_form(body: Bytes) -> Str {
  let r = http.new_request("POST", "http://h/p", {}, body)
  match r.form() {
    Ok(v) => fz_hx(v.encode())
    Err(e) => "err:${e.display()}"
  }
}
fn fz_text(status: Int, s: Str) -> Str {
  let r = http.text(status, s)
  fz_hx("${r.status}|${r.body.to_hex()}")
}
fn fz_json_resp(s: Str) -> Str {
  let r = http.json(201, s)
  fz_hx("${r.status}|${r.body.to_hex()}")
}
`

const templateHelpers = `
fn fz_tmpl(src: Str, data: Str) -> Str {
  match template.parse("t", src) {
    Ok(t) => match json.parse_value(data) {
      Ok(v) => match t.render(v) {
        Ok(s) => fz_hx(s)
        Err(e) => "err:${e.display()}"
      }
      Err(e) => "err:${e.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
fn fz_tmpl_missing(src: Str, data: Str) -> Str {
  match template.parse("t", src) {
    Ok(t) => match json.parse_value(data) {
      Ok(v) => match t.missing_key(error: true).render(v) {
        Ok(s) => fz_hx(s)
        Err(e) => "err:${e.display()}"
      }
      Err(e) => "err:${e.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
fn fz_tmpl_named(src: Str, src2: Str, data: Str) -> Str {
  match template.parse("t", src) {
    Ok(t) => match t.add("other", src2) {
      Ok(t2) => match json.parse_value(data) {
        Ok(v) => match t2.render_named("other", v) {
          Ok(s) => fz_hx(s)
          Err(e) => "err:${e.display()}"
        }
        Err(e) => "err:${e.display()}"
      }
      Err(e) => "err:${e.display()}"
    }
    Err(e) => "err:${e.display()}"
  }
}
`

const cryptoHelpers = `
fn fz_cop(op: Str, a: Bytes, b: Bytes, c: Bytes, d: Bytes) -> Str {
  internal.crypto_op(op, a, b, c, d).to_hex()
}
fn fz_seal_open(area: Str, key: Bytes, nonce: Bytes, pt: Bytes, aad: Bytes) -> Str {
  let sealed = internal.crypto_op("${area}.seal", key, nonce, pt, aad)
  let opened = internal.crypto_op("${area}.open", key, nonce, sealed, aad)
  let garbage = internal.crypto_op("${area}.open", key, nonce, b"not a sealed record", aad)
  "${sealed.to_hex()}|${opened.to_hex()}|${garbage.to_hex()}"
}
`

const x509Helpers = `
fn fz_xop(op: Str, a: Bytes, b: Bytes, c: Bytes, d: Bytes) -> Str {
  internal.crypto_op(op, a, b, c, d).to_hex()
}
fn fz_x509_plain(i: Int) -> Str {
  match Bytes.from_hex(CERT_HEX[i]) {
    Some(der) => fz_xop("x509.check", der, b"", b"", b"")
    None => "bad hex"
  }
}
fn fz_x509_mut(i: Int, pos: Int, nib: Str) -> Str {
  let h = CERT_HEX[i]
  let m = if pos < 1 || pos + 1 >= h.len() { h } else { h[0..pos] + nib + h[pos + 1..h.len()] }
  match Bytes.from_hex(m) {
    Some(der) => fz_xop("x509.check", der, b"", b"", b"")
    None => "bad hex"
  }
}
fn fz_x509_cut(i: Int, cut: Int) -> Str {
  let h = CERT_HEX[i]
  let m = h[0..(if cut < h.len() { cut } else { h.len() })]
  match Bytes.from_hex(m) {
    Some(der) => fz_xop("x509.check", der, b"", b"", b"")
    None => "bad hex"
  }
}
fn fz_x509_pair(i: Int, j: Int) -> Str {
  match Bytes.from_hex(CERT_HEX[i]) {
    Some(der) => match Bytes.from_hex(CERT_HEX[j]) {
      Some(other) => fz_xop("x509.signed_by", der, other, b"", b"")
      None => "bad hex"
    }
    None => "bad hex"
  }
}
`

const eccHelpers = `
fn fz_eop(curve: Str, what: Str, a: Bytes, b: Bytes, c: Bytes, d: Bytes) -> Str {
  internal.crypto_op("${curve}.${what}", a, b, c, d).to_hex()
}
fn fz_ecv(k: Int, h: Int, which: Int) -> Str {
  let curve = if k == 0 { "p256" } else { "p384" }
  let sig = if which == 0 { EC_SIG[h] } else if which == 1 { EC_SIG_BAD[h] } else { EC_SIG_SHORT[h] }
  match Bytes.from_hex(EC_PUB[k]) {
    Some(pk) => match Bytes.from_hex(EC_HASH[h]) {
      Some(hash) => match Bytes.from_hex(sig) {
        Some(s) => fz_eop(curve, "verify", pk, hash, s, b"")
        None => "bad hex"
      }
      None => "bad hex"
    }
    None => "bad hex"
  }
}
fn fz_ecpub(k: Int) -> Str {
  let curve = if k == 0 { "p256" } else { "p384" }
  match Bytes.from_hex(EC_PRIV[k]) {
    Some(priv) => fz_eop(curve, "public", priv, b"", b"", b"")
    None => "bad hex"
  }
}
fn fz_ecdh(k: Int) -> Str {
  let curve = if k == 0 { "p256" } else { "p384" }
  match Bytes.from_hex(EC_PRIV[k]) {
    Some(a) => match Bytes.from_hex(EC_PUB_B[k]) {
      Some(b) => match Bytes.from_hex(EC_PRIV_B[k]) {
        Some(c) => match Bytes.from_hex(EC_PUB[k]) {
          Some(d) => "${fz_eop(curve, "ecdh", a, b, b"", b"")},${fz_eop(curve, "ecdh", c, d, b"", b"")}"
          None => "bad hex"
        }
        None => "bad hex"
      }
      None => "bad hex"
    }
    None => "bad hex"
  }
}
fn fz_ecdh_bad(k: Int, cutn: Int) -> Str {
  let curve = if k == 0 { "p256" } else { "p384" }
  let peer = EC_PUB_B[k]
  let cut = if cutn * 2 < peer.len() { cutn * 2 } else { peer.len() }
  match Bytes.from_hex(EC_PRIV[k]) {
    Some(a) => match Bytes.from_hex(peer[0..cut]) {
      Some(b) => fz_eop(curve, "ecdh", a, b, b"", b"")
      None => "bad hex"
    }
    None => "bad hex"
  }
}
fn fz_rsa(kind: Int) -> Str {
  match Bytes.from_hex(RSA_PUB) {
    Some(pk) => match Bytes.from_hex(RSA_HASH) {
      Some(hash) => {
        let id = b"\x00"
        match kind {
          0 => fz_eop("rsa", "verify_pkcs1", id, pk, hash, Bytes.from_hex(RSA_SIG) ?? b"")
          1 => fz_eop("rsa", "verify_pkcs1", id, pk, hash, Bytes.from_hex(RSA_SIG_BAD) ?? b"")
          2 => fz_eop("rsa", "verify_pss", id, pk, hash, Bytes.from_hex(RSA_PSS) ?? b"")
          _ => fz_eop("rsa", "verify_pss", id, pk, hash, Bytes.from_hex(RSA_SIG) ?? b"")
        }
      }
      None => "bad hex"
    }
    None => "bad hex"
  }
}
fn fz_rsa_sign() -> Str {
  match Bytes.from_hex(RSA_PRIV) {
    Some(priv) => match Bytes.from_hex(RSA_HASH) {
      Some(hash) => {
        let sig = fz_eop("rsa", "sign_pkcs1", b"\x00", priv, hash, b"")
        if sig == RSA_SIG { "same as Go" } else { "differs ${sig}" }
      }
      None => "bad hex"
    }
    None => "bad hex"
  }
}
`

const limitsHelpers = `
fn fz_fields(rows: List[List[Str]]) -> Int {
  var t = 0
  for r in rows {
    for f in r {
      t += f.len()
    }
  }
  t
}
fn fz_lim(kind: Int, pat: Str, n: Int) -> Str {
  let s = pat.repeat(n)
  let b = s.bytes()
  match kind {
    0 => "${s.len()}/${s.rune_count()}"
    1 => "${s.to_lower().len()}/${s.to_upper().len()}"
    2 => "${s.find("a") ?? -1}/${s.rfind("a") ?? -1}"
    3 => "${s.replace("a", "xy").len()}"
    4 => "${s.trim().len()}/${s.trim_start_chars("a").len()}"
    5 => "${b.len()}/${b.rune_count()}/${b.find(b"a") ?? -1}"
    6 => "${base64.encode(b).len()}/${(base64.decode(base64.encode(b)).ok() ?? b"").len()}"
    7 => "${base32.encode(b).len()}/${(base32.decode(base32.encode(b)).ok() ?? b"").len()}"
    8 => "${compress.gzip(b).len()}/${(compress.gunzip(compress.gzip(b)).ok() ?? b"").len()}"
    9 => match json.parse_value(json.encode_str(s)) {
      Ok(v) => "${json.encode_str(s).len()}/${v.encode().len()}"
      Err(e) => "err:${e.display()}"
    }
    10 => match csv.parse("a,${s},b") {
      Ok(rows) => "${rows.len()}/${fz_fields(rows)}"
      Err(e) => "err:${e.display()}"
    }
    11 => {
      let esc = url.query_escape(s)
      let back = url.query_unescape(esc).ok() ?? ""
      "${esc.len()}/${back.len()}"
    }
    _ => "${b.split(b"a").len()}/${b.split(b"a")[0].len()}"
  }
}
`

// A fuzzCase is one line of the generated program: the name it prints and
// the Xo expression that computes its value as a Str, so every result
// compares as one line of text.
type fuzzCase struct {
	Name string // area.0001
	Expr string // already rendered to a Str
}

func (c fuzzCase) stmt() string {
	return fmt.Sprintf("  fz_emit(os.stdio, %s, %s)", xq(c.Name), c.Expr)
}

// An area is one std module's functions, the inputs it draws, and the Xo
// imports it needs.
type area struct {
	Name    string
	Uses    []string
	Helpers string
	Gen     func(r *rand.Rand, n int) []fuzzCase
}

func allAreas() []area {
	return []area{
		{"strings", nil, "", genStrings},
		{"bytes", nil, "", genBytes},
		{"strconv", nil, "", genStrconv},
		{"json", []string{"std/json"}, jsonHelpers, genJSON},
		{"path", []string{"std/path"}, "", genPath},
		{"url", []string{"std/url"}, urlHelpers, genURL},
		{"base64", []string{"std/base64", "std/base32"}, baseHelpers, genBase},
		{"time", []string{"std/time"}, timeHelpers, genTime},
		{"regexp", []string{"std/regexp"}, regexpHelpers, genRegexp},
		{"csv", []string{"std/csv"}, csvHelpers, genCSV},
		{"netip", []string{"std/netip"}, netipHelpers, genNetip},
		{"big", []string{"std/big"}, bigHelpers, genBig},
		{"compress", []string{"std/compress"}, compressHelpers, genCompress},
		{"archive", []string{"std/archive"}, archiveHelpers, genArchive},
		{"mime", []string{"std/mime"}, mimeHelpers, genMime},
		{"multipart", []string{"std/multipart"}, multipartHelpers, genMultipart},
		{"xml", []string{"std/xml"}, xmlHelpers, genXML},
		{"sql", []string{"std/sql"}, sqlHelpers, genSQL},
		{"http", []string{"std/http"}, httpHelpers, genHTTP},
		{"template", []string{"std/template", "std/json"}, templateHelpers, genTemplate},
		{"cryptostream", []string{"std/internal"}, cryptoHelpers, genCrypto},
		{"x509", []string{"std/internal"}, x509Helpers, genX509},
		{"ecc", []string{"std/internal"}, eccHelpers, genECC},
		{"limits", []string{"std/base32", "std/base64", "std/compress", "std/csv", "std/json", "std/url"}, limitsHelpers, genLimits},
	}
}

// pool is the fixed part of every area's inputs, so a seed still meets the
// empty, separator, unicode and control cases.
var pool = []string{
	"", "a", "abc", "  x  ", "a,b,,c", "aaa", " Hello, World ", "héllo",
	"a\nb\n", "\t x \t", "ABC", "İstanbul", "ß", "🚀x🚀", "ab\u00a0cd",
	"  ", "a.b.c", "-1e5", "0x1F", "..", "\u2028x", "\"quoted\"", "a\\b",
	"line1\r\nline2", "aaa\taaa", "$var", "x\x00y", " ",
}

// subs are the separators, prefixes and cutsets cases draw from.
var subs = []string{"", "a", "b", "ab", ",", " ", "x", "aa", "é", "🚀", "\n", ".", "aaa", "Hé"}

func pick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

// randStr draws a string: a pool entry, a random rune mix, or a long one.
func randStr(r *rand.Rand) string {
	switch r.Intn(3) {
	case 0:
		return pick(r, pool)
	case 1:
		var b strings.Builder
		alpha := []rune("ab c,\t\nABCéß🚀012.-_/\\\"'$")
		for i, n := 0, r.Intn(14); i < n; i++ {
			b.WriteRune(alpha[r.Intn(len(alpha))])
		}
		return b.String()
	}
	var b strings.Builder
	for i := 0; i < 3*r.Intn(20); i++ {
		b.WriteString(pick(r, pool))
	}
	return b.String()
}

// randBytes draws arbitrary bytes, invalid UTF-8 included: Bytes may hold
// what Str cannot.
func randBytes(r *rand.Rand) []byte {
	if r.Intn(3) == 0 {
		return []byte(pick(r, pool))
	}
	b := make([]byte, r.Intn(18))
	for i := range b {
		b[i] = byte(r.Intn(256))
	}
	return b
}

// genStrings covers Str's pure methods.
func genStrings(r *rand.Rand, n int) []fuzzCase {
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		s, sub, cut := randStr(r), pick(r, subs), pick(r, subs)
		S := xq(s)
		name := fmt.Sprintf("strings.%04d", i)
		k := r.Intn(4)
		switch i % 40 {
		case 0:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim())"})
		case 1:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_start())"})
		case 2:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_end())"})
		case 3:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_chars(" + xq(cut) + "))"})
		case 4:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_start_chars(" + xq(cut) + "))"})
		case 5:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_end_chars(" + xq(cut) + "))"})
		case 6:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".to_lower())"})
		case 7:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".to_upper())"})
		case 8:
			out = append(out, fuzzCase{name, "fz_opt_i(" + S + ".find(" + xq(sub) + "))"})
		case 9:
			out = append(out, fuzzCase{name, "fz_opt_i(" + S + ".rfind(" + xq(sub) + "))"})
		case 10:
			out = append(out, fuzzCase{name, "fz_opt_i(" + S + ".find_any(" + xq(cut) + "))"})
		case 11:
			out = append(out, fuzzCase{name, "fz_opt_i(" + S + ".rfind_any(" + xq(cut) + "))"})
		case 12:
			out = append(out, fuzzCase{name, "fz_ls(" + S + ".split(" + xq(sub) + "))"})
		case 13:
			out = append(out, fuzzCase{name, "fz_ls(" + S + ".split_n(" + xq(sub) + ", " + fmt.Sprint(k) + "))"})
		case 14:
			out = append(out, fuzzCase{name, "fz_ls(" + S + ".split_after(" + xq(sub) + "))"})
		case 15:
			out = append(out, fuzzCase{name, "fz_ls(" + S + ".fields())"})
		case 16:
			out = append(out, fuzzCase{name, "fz_ls(" + S + ".lines())"})
		case 17:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".replace(" + xq(sub) + ", " + xq(cut) + "))"})
		case 18:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".replace_n(" + xq(sub) + ", " + xq(cut) + ", " + fmt.Sprint(k) + "))"})
		case 19:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".repeat(" + fmt.Sprint(k) + "))"})
		case 20:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_prefix(" + xq(sub) + "))"})
		case 21:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_suffix(" + xq(sub) + "))"})
		case 22:
			out = append(out, fuzzCase{name, "fz_opt_s(" + S + ".cut_prefix(" + xq(sub) + "))"})
		case 23:
			out = append(out, fuzzCase{name, "fz_opt_s(" + S + ".cut_suffix(" + xq(sub) + "))"})
		case 24:
			out = append(out, fuzzCase{name, "fz_bool(" + S + ".contains(" + xq(sub) + "))"})
		case 25:
			out = append(out, fuzzCase{name, "fz_bool(" + S + ".starts_with(" + xq(sub) + "))"})
		case 26:
			out = append(out, fuzzCase{name, "fz_bool(" + S + ".ends_with(" + xq(sub) + "))"})
		case 27:
			out = append(out, fuzzCase{name, "fz_bool(" + S + ".equal_fold(" + xq(sub) + "))"})
		case 28:
			out = append(out, fuzzCase{name, "fz_bool(" + S + ".contains_any(" + xq(cut) + "))"})
		case 29:
			out = append(out, fuzzCase{name, "fz_i(" + S + ".count(" + xq(sub) + "))"})
		case 30:
			out = append(out, fuzzCase{name, "fz_i(" + S + ".len())"})
		case 31:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_bools(%s.is_empty(), %s.contains_rune('a'), %s.runes().len() == %s.rune_count(), true, false)",
				S, S, S, S)})
		case 32:
			out = append(out, fuzzCase{name, "fz_opt_i(" + S + ".find_where(fz_is_letter))"})
		case 33:
			out = append(out, fuzzCase{name, "fz_opt_i(" + S + ".rfind_where(fz_is_digit))"})
		case 34:
			out = append(out, fuzzCase{name, "fz_bool(" + S + ".contains_where(fz_is_letter))"})
		case 35:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_where(fz_is_digit))"})
		case 36:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_start_where(fz_is_letter))"})
		case 37:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_end_where(fz_is_letter))"})
		case 38:
			out = append(out, fuzzCase{name, "fz_ls(" + S + ".split_where(fz_is_letter))"})
		case 39:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".map_runes(fz_upper_keep))"})
		}
	}
	return out
}

// genBytes covers Bytes' pure methods.
func genBytes(r *rand.Rand, n int) []fuzzCase {
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		b, sub, cut := randBytes(r), randBytes(r), pick(r, subs)
		B, SUB := xbytes(b), xbytes(sub)
		name := fmt.Sprintf("bytes.%04d", i)
		k := r.Intn(4)
		switch i % 32 {
		case 0:
			out = append(out, fuzzCase{name, B + ".to_hex()"})
		case 1:
			out = append(out, fuzzCase{name, "fz_hx(" + B + ".to_base64())"})
		case 2:
			out = append(out, fuzzCase{name, "fz_opt_s(" + B + ".to_str())"})
		case 3:
			out = append(out, fuzzCase{name, "fz_opt_i(" + B + ".find(" + SUB + "))"})
		case 4:
			out = append(out, fuzzCase{name, "fz_opt_i(" + B + ".rfind(" + SUB + "))"})
		case 5:
			out = append(out, fuzzCase{name, "fz_opt_i(" + B + ".find_any(" + xq(cut) + "))"})
		case 6:
			out = append(out, fuzzCase{name, "fz_lb(" + B + ".split(" + SUB + "))"})
		case 7:
			out = append(out, fuzzCase{name, "fz_lb(" + B + ".split_n(" + SUB + ", " + fmt.Sprint(k) + "))"})
		case 8:
			out = append(out, fuzzCase{name, "fz_lb(" + B + ".split_after(" + SUB + "))"})
		case 9:
			out = append(out, fuzzCase{name, "fz_lb(" + B + ".fields())"})
		case 10:
			out = append(out, fuzzCase{name, B + ".trim().to_hex()"})
		case 11:
			out = append(out, fuzzCase{name, B + ".trim_prefix(" + SUB + ").to_hex()"})
		case 12:
			out = append(out, fuzzCase{name, B + ".trim_suffix(" + SUB + ").to_hex()"})
		case 13:
			out = append(out, fuzzCase{name, "fz_opt_b(" + B + ".cut_prefix(" + SUB + "))"})
		case 14:
			out = append(out, fuzzCase{name, "fz_opt_b(" + B + ".cut_suffix(" + SUB + "))"})
		case 15:
			out = append(out, fuzzCase{name, B + ".replace(" + SUB + ", " + xbytes([]byte(cut)) + ").to_hex()"})
		case 16:
			out = append(out, fuzzCase{name, B + ".replace_n(" + SUB + ", " + xbytes([]byte(cut)) + ", " + fmt.Sprint(k) + ").to_hex()"})
		case 17:
			out = append(out, fuzzCase{name, B + ".to_lower().to_hex()"})
		case 18:
			out = append(out, fuzzCase{name, B + ".to_upper().to_hex()"})
		case 19:
			out = append(out, fuzzCase{name, B + ".repeat(" + fmt.Sprint(k) + ").to_hex()"})
		case 20:
			out = append(out, fuzzCase{name, B + ".clone().to_hex()"})
		case 21:
			out = append(out, fuzzCase{name, "fz_bool(" + B + ".contains(" + SUB + "))"})
		case 22:
			out = append(out, fuzzCase{name, "fz_bool(" + B + ".starts_with(" + SUB + "))"})
		case 23:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_bools(%s.ends_with(%s), %s.equal_fold(%s), %s.contains_any(%s), %s.is_empty(), false)",
				B, SUB, xbytes([]byte(cut)), xbytes([]byte(cut)), B, xq(cut), B)})
		case 24:
			out = append(out, fuzzCase{name, "fz_opt_i(" + B + ".find_where(fz_is_letter))"})
		case 25:
			out = append(out, fuzzCase{name, "fz_opt_i(" + B + ".rfind_where(fz_is_digit))"})
		case 26:
			out = append(out, fuzzCase{name, "fz_bool(" + B + ".contains_where(fz_is_letter))"})
		case 27:
			out = append(out, fuzzCase{name, B + ".trim_where(fz_is_digit).to_hex()"})
		case 28:
			out = append(out, fuzzCase{name, B + ".trim_start_where(fz_is_letter).to_hex()"})
		case 29:
			out = append(out, fuzzCase{name, B + ".trim_end_where(fz_is_letter).to_hex()"})
		case 30:
			out = append(out, fuzzCase{name, "fz_lb(" + B + ".split_where(fz_is_letter))"})
		case 31:
			out = append(out, fuzzCase{name, B + ".map_runes(fz_upper_keep).to_hex()"})
		}
	}
	return out
}

// genStrconv covers Int, Float and Bool parsing and formatting.
func genStrconv(r *rand.Rand, n int) []fuzzCase {
	ints := []string{"0", "1", "-1", "42", "-42", "9223372036854775807", "-9223372036854775808",
		"9223372036854775808", "007", "+7", " 7", "7 ", "0x1F", "1_000", "", "-", "12a", "1e3", "٣"}
	floats := []string{"0", "-0", "1", "-1", "1.5", ".5", "5.", "1e10", "1E-10", "1e999",
		"-1e999", "NaN", "nan", "Inf", "+Inf", "-Inf", "infinity", "0x1p-2", "3.141592653589793238",
		"1.7976931348623157e308", "5e-324", "4.9e-324", "", " ", "1,5", "1e", "e1", "+.5", "- 1"}
	bools := []string{"1", "0", "t", "f", "T", "F", "TRUE", "true", "True", "false", "False", "yes", "", "2", " true", "TRUE "}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("strconv.%04d", i)
		base := []int{2, 8, 10, 16, 36}[r.Intn(5)]
		switch i % 10 {
		case 0:
			out = append(out, fuzzCase{name, "fz_opt_i(Int.parse(" + xq(pick(r, ints)) + "))"})
		case 1:
			out = append(out, fuzzCase{name, "fz_opt_i(Int.parse_base(" + xq(pick(r, ints)) + ", " + fmt.Sprint(base) + "))"})
		case 2:
			out = append(out, fuzzCase{name, "fz_hx((" + fmt.Sprint(r.Int63()-r.Int63()) + ").to_str_base(" + fmt.Sprint(base) + "))"})
		case 3:
			out = append(out, fuzzCase{name, "fz_opt_f(Float.parse(" + xq(pick(r, floats)) + "))"})
		case 4:
			out = append(out, fuzzCase{name, "fz_opt_bool(Bool.parse(" + xq(pick(r, bools)) + "))"})
		case 5:
			out = append(out, fuzzCase{name, "fz_i(" + fmt.Sprint(r.Int63()-r.Int63()) + ")"})
		case 6:
			out = append(out, fuzzCase{name, "fz_f(Float.from_bits(" + fmt.Sprint(int64(r.Uint64())) + "))"})
		case 7:
			out = append(out, fuzzCase{name, "fz_bool(Float.from_bits(" + fmt.Sprint(int64(r.Uint64())) + ").is_nan())"})
		case 8:
			out = append(out, fuzzCase{name, "fz_opt_i(Int.parse(" + xq(fmt.Sprint(r.Int63())) + "))"})
		case 9:
			out = append(out, fuzzCase{name, "fz_opt_f(Float.parse(" + xq(fmt.Sprintf("%v", r.NormFloat64()*math.Pow(10, float64(r.Intn(40)-20)))) + "))"})
		}
	}
	return out
}

// genJSON covers json.parse_value and Value.encode over numbers, escapes,
// nesting and junk.
func genJSON(r *rand.Rand, n int) []fuzzCase {
	atoms := []string{"null", "true", "false", "0", "-0", "1", "-1", "1.5", "1e21", "1e-7",
		"123456789012345678901234567890", "-9223372036854775808", "9223372036854775807",
		"1e999", "0.1", "\"\"", "\"a\"", "\"a\\\"b\"", "\"tab\\there\"", "\"\\u00e9\"",
		"\"\\ud834\\udd1e\"", "\"\\ud834\"", "\"a\nb\"", "\"\\u0000\"", "[]", "{}",
		"{\"a\":1}", "{\"a\":1,\"b\":[true,null]}", "{\"a\":1,\"a\":2}", "[1,2,3]",
		"{\"z\":1,\"a\":2}", "{", "[1,", "tru", "01", "+1", ".5", "1.", "NaN", " ", "",
		"[1 2]", "{\"a\" 1}", "{\"a\":}", "\"\x01\"", "{\"a\":[{\"b\":{\"c\":[]}}]}"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("json.%04d", i)
		var text string
		switch i % 3 {
		case 0:
			text = pick(r, atoms)
		case 1:
			depth := 1 + r.Intn(40)
			text = strings.Repeat("[", depth) + strings.Repeat("]", depth)
		case 2:
			var b strings.Builder
			b.WriteString("{")
			for k, m := 0, 1+r.Intn(4); k < m; k++ {
				if k > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, "%q:%d", fmt.Sprintf("k%d", r.Intn(6)), r.Intn(1000)-500)
			}
			b.WriteString("}")
			text = b.String()
		}
		out = append(out, fuzzCase{name, "fz_json(" + xq(text) + ")"})
	}
	return out
}

// genPath covers std/path's pure functions.
func genPath(r *rand.Rand, n int) []fuzzCase {
	paths := []string{"", ".", "..", "/", "//", "/a", "/a/b", "a", "a/", "a//b", "./a", "../a",
		"a/../b", "a/./b", "/a/../../b", "C:\\a\\b", "C:/a/b", "a/b/", ".hidden", "a.b.c",
		"a/b.ext", "/usr/lib/libx.so.1", " ", "a b/c d", "/a/", "//host/share", "a\x00b"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		a, b := pick(r, paths), pick(r, paths)
		name := fmt.Sprintf("path.%04d", i)
		switch i % 6 {
		case 0:
			out = append(out, fuzzCase{name, "fz_hx(path.clean(" + xq(a) + "))"})
		case 1:
			out = append(out, fuzzCase{name, "fz_hx(path.join(" + xq(a) + ", " + xq(b) + "))"})
		case 2:
			out = append(out, fuzzCase{name, "fz_hx(path.base(" + xq(a) + "))"})
		case 3:
			out = append(out, fuzzCase{name, "fz_hx(path.dir(" + xq(a) + "))"})
		case 4:
			out = append(out, fuzzCase{name, "fz_hx(path.ext(" + xq(a) + "))"})
		case 5:
			out = append(out, fuzzCase{name, "fz_bool(path.is_abs(" + xq(a) + "))"})
		}
	}
	return out
}

// genURL covers std/url's escapes and query parsing.
func genURL(r *rand.Rand, n int) []fuzzCase {
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		s := randStr(r)
		if i%4 == 3 {
			s = pick(r, []string{"%", "%2", "%zz", "a%20b", "+", "a+b", "%e2%9c%93", "%ff", "a b&c=d/é", "?x=1&y=2"})
		}
		name := fmt.Sprintf("url.%04d", i)
		switch i % 6 {
		case 0:
			out = append(out, fuzzCase{name, "fz_hx(url.query_escape(" + xq(s) + "))"})
		case 1:
			out = append(out, fuzzCase{name, "fz_hx(url.path_escape(" + xq(s) + "))"})
		case 2:
			out = append(out, fuzzCase{name, "fz_un_q(" + xq(s) + ")"})
		case 3:
			out = append(out, fuzzCase{name, "fz_un_p(" + xq(s) + ")"})
		case 4:
			out = append(out, fuzzCase{name, "fz_query(" + xq(s) + ")"})
		case 5:
			out = append(out, fuzzCase{name, "fz_hx(url.values().encode())"})
		}
	}
	return out
}

// genBase covers base64, base32 and hex.
func genBase(r *rand.Rand, n int) []fuzzCase {
	texts := []string{"", "A", "AA", "AAA", "AAAA", "aGVsbG8=", "aGVsbG8", "!!!!", "a b", "a\nb",
		"AB==", "A===", "abcd", "-_-_", "+//+", "0O1l", "MZXW6===", "MZXW6YQ=", "MZXW6YTB"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		b := randBytes(r)
		B := xbytes(b)
		name := fmt.Sprintf("base64.%04d", i)
		switch i % 8 {
		case 0:
			out = append(out, fuzzCase{name, "fz_hx(base64.encode(" + B + ", base64.Base64.Std))"})
		case 1:
			out = append(out, fuzzCase{name, "fz_hx(base64.encode(" + B + ", base64.Base64.Url))"})
		case 2:
			out = append(out, fuzzCase{name, "fz_hx(base64.encode(" + B + ", base64.Base64.RawStd))"})
		case 3:
			out = append(out, fuzzCase{name, "fz_b64(" + xq(pick(r, texts)) + ", base64.Base64.Std)"})
		case 4:
			out = append(out, fuzzCase{name, "fz_b64(" + xq(pick(r, texts)) + ", base64.Base64.RawUrl)"})
		case 5:
			out = append(out, fuzzCase{name, "fz_hx(base32.encode(" + B + ", base32.Base32.Std))"})
		case 6:
			out = append(out, fuzzCase{name, "fz_b32(" + xq(pick(r, texts)) + ")"})
		case 7:
			out = append(out, fuzzCase{name, "fz_opt_b(Bytes.from_hex(" + xq(pick(r, texts)) + "))"})
		}
	}
	return out
}

// genTime covers Duration parsing and display, RFC3339 parsing and
// formatting, and formatting a Time at UTC and at a fixed zone.
func genTime(r *rand.Rand, n int) []fuzzCase {
	durs := []string{"0", "1s", "1.5s", "-2h45m", "300ms", "1us", "1µs", "1ns", "1m", "1h",
		"", "s", "1", "1 s", " 1s ", "+1s", "1.5h", "0.5m", "1d", "1S", "1e3s", "2562047h",
		"9223372036854775807ns", "-9223372036854775808ns", "1h1m1s1ms1us1ns"}
	stamps := []string{"1970-01-01T00:00:00Z", "2026-10-24T12:34:56Z", "2026-10-24T12:34:56.123456789Z",
		"2026-10-24T12:34:56+02:00", "2026-10-24T12:34:56-07:30", "2026-02-29T00:00:00Z",
		"2024-02-29T23:59:59Z", "0000-01-01T00:00:00Z", "9999-12-31T23:59:59Z",
		"2026-10-24T24:00:00Z", "2026-10-24t12:34:56z", "2026-10-24T12:34:56", "2026-10-24T12:34:56.Z",
		"2026-10-24T12:34:56.1Z", "2026-10-24T12:34:56+24:00", "2026-10-24T12:34:56+23:59",
		"2026-10-24 12:34:56Z", "", "not a time", "2026-13-01T00:00:00Z"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("time.%04d", i)
		switch i % 6 {
		case 0:
			out = append(out, fuzzCase{name, "fz_dur(" + xq(pick(r, durs)) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_hx(Duration.from_nanos(" + fmt.Sprint(r.Int63()-r.Int63()) + ").display())"})
		case 2:
			out = append(out, fuzzCase{name, "fz_stamp(" + xq(pick(r, stamps)) + ")"})
		case 3:
			out = append(out, fuzzCase{name, "fz_hx(time.format(Time.from_unix(" + fmt.Sprint(r.Int63n(2000000000)-1000000000) + "), time.utc(), time.RFC3339))"})
		case 4:
			out = append(out, fuzzCase{name, "fz_hx(time.format(Time.from_unix_millis(" + fmt.Sprint(r.Int63n(2000000000000)-1000000000000) + "), time.utc(), time.DATE))"})
		case 5:
			off := r.Intn(47*3600) - 23*3600
			out = append(out, fuzzCase{name, "fz_hx(time.format(Time.from_unix(" + fmt.Sprint(r.Int63n(2000000000)) + "), time.fixed(\"X\", Duration.from_secs_int(" + fmt.Sprint(off) + ")), time.RFC3339))"})
		}
	}
	return out
}

// genRegexp covers compile, match, find, find_all, split and replace_all.
func genRegexp(r *rand.Rand, n int) []fuzzCase {
	pats := []string{"a", "a+", "^a", "a$", "(a)(b)?", "[a-c]+", "[^x]", "\\d+", "\\s", "(?i)a",
		"a|b", ".", "a{2,3}", "(?P<n>a)", "\\b", "^$", "[", "(a", "*x", "a**", "\\p{L}", "é", "\\p{Han}"}
	repls := []string{"", "X", "$1", "${n}", "a$", "\\n"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		p, s := pick(r, pats), randStr(r)
		name := fmt.Sprintf("regexp.%04d", i)
		switch i % 6 {
		case 0:
			out = append(out, fuzzCase{name, "fz_re_m(" + xq(p) + ", " + xq(s) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_re_f(" + xq(p) + ", " + xq(s) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_re_a(" + xq(p) + ", " + xq(s) + ")"})
		case 3:
			out = append(out, fuzzCase{name, "fz_re_sp(" + xq(p) + ", " + xq(s) + ")"})
		case 4:
			out = append(out, fuzzCase{name, "fz_re_r(" + xq(p) + ", " + xq(s) + ", " + xq(pick(r, repls)) + ")"})
		case 5:
			out = append(out, fuzzCase{name, "fz_hx(regexp.quote(" + xq(s) + "))"})
		}
	}
	return out
}

// csvText draws CSV-ish text: quoted fields, embedded separators and
// newlines, comments, blank lines and broken quoting.
func csvText(r *rand.Rand) string {
	if r.Intn(3) == 0 {
		return pick(r, []string{"", "a", "a,b", "a,b\nc,d", "\"a\",\"b\"", "a,\"b,c\",d",
			"#c\na,b", "a;b", "a,\"b", "\"a\"x,b", " a , b ", "a\r\nb", "\n\n", ",\n,,"})
	}
	fields := []string{"a", "b", "x,y", "q\"r", " lead", "trail ", "é", "🚀", "", ";"}
	var b strings.Builder
	for i, rows := 0, r.Intn(4); i < rows; i++ {
		for j, cols := 0, 1+r.Intn(4); j < cols; j++ {
			if j > 0 {
				b.WriteString(pick(r, []string{",", ";", ",\"", "\""}))
			}
			f := pick(r, fields)
			if r.Intn(4) == 0 {
				b.WriteString("\"" + f + "\"")
			} else {
				b.WriteString(f)
			}
		}
		b.WriteString(pick(r, []string{"\n", "\r\n", "\n#c\n"}))
	}
	return b.String()
}

// genCSV covers parse with its options and format.
func genCSV(r *rand.Rand, n int) []fuzzCase {
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		s := csvText(r)
		name := fmt.Sprintf("csv.%04d", i)
		switch i % 4 {
		case 0:
			out = append(out, fuzzCase{name, "fz_csv(" + xq(s) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_csv_semi(" + xq(s) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_csv_lazy(" + xq(s) + ")"})
		case 3:
			rows := fmt.Sprintf("[[%s, %s], [%s, %s]]", xq(pick(r, pool)), xq(pick(r, subs)),
				xq(pick(r, []string{"a\"b", "c,d", " e"})), xq(pick(r, subs)))
			out = append(out, fuzzCase{name, "fz_hx(csv.format(" + rows + ", crlf: " + fmt.Sprint(r.Intn(2) == 0) + "))"})
		}
	}
	return out
}

// genNetip covers address and prefix parsing, display and predicates.
func genNetip(r *rand.Rand, n int) []fuzzCase {
	addrs := []string{"1.2.3.4", "0.0.0.0", "255.255.255.255", "127.0.0.1", "192.168.1.1",
		"10.0.0.1", "169.254.1.1", "224.0.0.1", "::1", "::", "2001:db8::1", "fe80::1",
		"::ffff:1.2.3.4", "64:ff9b::1.2.3.4", "1.2.3", "01.2.3.4", "1.2.3.4.5", "::g",
		"2001:db8::1%eth0", "", "1.2.3.4 ", " 1.2.3.4", "[::1]"}
	prefixes := []string{"10.0.0.0/8", "192.168.0.0/16", "0.0.0.0/0", "1.2.3.4/32", "1.2.3.4/33",
		"2001:db8::/32", "::/0", "::1/128", "1.2.3.4/-1", "10.0.0.0", "10.0.0.0/8/9", ""}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("netip.%04d", i)
		switch i % 3 {
		case 0:
			out = append(out, fuzzCase{name, "fz_addr(" + xq(pick(r, addrs)) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_addr6(" + xq(pick(r, addrs)) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_pfx(" + xq(pick(r, prefixes)) + ")"})
		}
	}
	return out
}

// genBig covers big.parse and the arithmetic, bitwise and compare methods.
func genBig(r *rand.Rand, n int) []fuzzCase {
	dec := []string{"0", "1", "-1", "42", "-42", "9223372036854775807",
		"-9223372036854775808", "123456789012345678901234567890",
		"-123456789012345678901234567890", "0007", "+7", " 7", "7 ", "", "-", "12a",
		"1e3", "ffffffffffffffffffffffff", "-0"}
	hex := []string{"0", "1", "ff", "-ff", "deadBEEF", "7fffffffffffffffffffffffffffffff",
		"-80000000000000000000000000000000", "g", ""}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("big.%04d", i)
		switch i % 3 {
		case 0:
			out = append(out, fuzzCase{name, "fz_big_parse(" + xq(pick(r, dec)) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_big_hex(" + xq(pick(r, hex)) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_big_ops(" + fmt.Sprint(r.Int63()-r.Int63()) + ", " + fmt.Sprint(r.Int63()-r.Int63()) + ")"})
		}
	}
	return out
}

// genCompress covers gzip, deflate and zlib, the round trips, the
// streaming compressor and input a decoder must refuse.
func genCompress(r *rand.Rand, n int) []fuzzCase {
	goLevels := []int{flate.DefaultCompression, flate.BestSpeed, flate.BestCompression, flate.NoCompression, flate.HuffmanOnly}
	levels := []string{"compress.Level.Default", "compress.Level.BestSpeed",
		"compress.Level.BestCompression", "compress.Level.NoCompression", "compress.Level.HuffmanOnly"}
	formats := []string{"compress.Format.Gzip", "compress.Format.Deflate", "compress.Format.Zlib"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		b := randBytes(r)
		if r.Intn(4) == 0 {
			b = []byte(strings.Repeat(pick(r, pool), 1+r.Intn(200)))
		}
		B := xbytes(b)
		name := fmt.Sprintf("compress.%04d", i)
		switch i % 10 {
		case 0:
			out = append(out, fuzzCase{name, "fz_gz(" + B + ", " + pick(r, levels) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_gz_rt(" + B + ", " + pick(r, levels) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_deflate_rt(" + B + ")"})
		case 3:
			out = append(out, fuzzCase{name, "fz_zlib_rt(" + B + ")"})
		case 4:
			out = append(out, fuzzCase{name, "fz_gunzip(" + B + ")"})
		case 5:
			out = append(out, fuzzCase{name, "fz_stream(" + B + ", " + pick(r, formats) + ")"})
		case 6:
			out = append(out, fuzzCase{name, "fz_hx(compress.deflate(" + B + ", " + pick(r, levels) + ").to_hex())"})
		case 7:
			out = append(out, fuzzCase{name, "fz_gunzip(" + xbytes(goGzip(b, goLevels[r.Intn(len(goLevels))])) + ")"})
		case 8:
			out = append(out, fuzzCase{name, "fz_inflate(" + xbytes(goDeflate(b, goLevels[r.Intn(len(goLevels))])) + ")"})
		case 9:
			out = append(out, fuzzCase{name, "fz_unzlib(" + xbytes(goZlib(b, goLevels[r.Intn(len(goLevels))])) + ")"})
		}
	}
	return out
}

// genArchive covers tar, zip and gzip archives: the bytes written, the
// entries read back, and input an archive reader must refuse.
func genArchive(r *rand.Rand, n int) []fuzzCase {
	names := []string{"a.txt", "dir/b", "", "a", "a b", "é.txt", "a/b/c", "/abs", "../x", "a\x00b"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		b, name := randBytes(r), pick(r, names)
		B, N := xbytes(b), xq(name)
		switch i % 9 {
		case 0:
			out = append(out, fuzzCase{fmt.Sprintf("archive.%04d", i), "fz_tar(" + N + ", " + B + ")"})
		case 1:
			out = append(out, fuzzCase{fmt.Sprintf("archive.%04d", i), "fz_zip(" + N + ", " + B + ")"})
		case 2:
			out = append(out, fuzzCase{fmt.Sprintf("archive.%04d", i), "fz_untar(" + N + ", " + B + ")"})
		case 3:
			out = append(out, fuzzCase{fmt.Sprintf("archive.%04d", i), "fz_unzip(" + N + ", " + B + ")"})
		case 4:
			out = append(out, fuzzCase{fmt.Sprintf("archive.%04d", i), "fz_untar_raw(" + B + ")"})
		case 5:
			out = append(out, fuzzCase{fmt.Sprintf("archive.%04d", i), "fz_agunzip(" + B + ")"})
		case 6:
			gname := pick(r, []string{"a.txt", "dir/b", "dir/a/b.txt", "x", "a b.txt"})
			out = append(out, fuzzCase{fmt.Sprintf("archive.%04d", i), "fz_untar_raw(" + xbytes(goTar(gname, b)) + ")"})
		case 7:
			gname := pick(r, []string{"a.txt", "dir/b", "dir/a/b.txt", "x", "a b.txt"})
			out = append(out, fuzzCase{fmt.Sprintf("archive.%04d", i), "fz_unzip_raw(" + xbytes(goZip(gname, b)) + ")"})
		case 8:
			// Go's zip writer accepts a NUL in a name; every backend's
			// unzip must refuse it with "bad entry path" (decision 0164's
			// path rules, the tar NUL fix of 2026-10-24).
			zname := pick(r, []string{"a\x00b", "a\x00b.txt", "d/a\x00b"})
			out = append(out, fuzzCase{fmt.Sprintf("archive.%04d", i), "fz_unzip_raw(" + xbytes(goZip(zname, b)) + ")"})
		}
	}
	return out
}

// The payloads below are written by Go's own packages at generation time,
// so the Xo decoders are checked against Go's writers, not only against
// Xo's.

func goGzip(data []byte, level int) []byte {
	var b bytes.Buffer
	w, err := gzip.NewWriterLevel(&b, level)
	if err != nil {
		panic(err)
	}
	if _, err := w.Write(data); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func goDeflate(data []byte, level int) []byte {
	var b bytes.Buffer
	w, err := flate.NewWriter(&b, level)
	if err != nil {
		panic(err)
	}
	if _, err := w.Write(data); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func goZlib(data []byte, level int) []byte {
	var b bytes.Buffer
	w, err := zlib.NewWriterLevel(&b, level)
	if err != nil {
		panic(err)
	}
	if _, err := w.Write(data); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func goTar(name string, data []byte) []byte {
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	h := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), ModTime: time.Unix(0, 0)}
	if err := w.WriteHeader(h); err != nil {
		panic(err)
	}
	if _, err := w.Write(data); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func goZip(name string, data []byte) []byte {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: name, Method: zip.Deflate}
	h.SetModTime(time.Unix(0, 0))
	f, err := w.CreateHeader(h)
	if err != nil {
		panic(err)
	}
	if _, err := f.Write(data); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// genMime covers media type parsing and formatting, the extension table
// and content sniffing.
func genMime(r *rand.Rand, n int) []fuzzCase {
	types := []string{"text/html", "text/html; charset=utf-8", "TEXT/HTML; CHARSET=UTF-8",
		"text/plain; charset=\"utf-8\"", "text/plain; charset=utf-8; boundary=x",
		"multipart/form-data; boundary=--abc", "application/json", "a/b",
		"text/plain; charset*=utf-8''a%20b", "text/plain; title*0*=utf-8''a; title*1*=b",
		"", " ", "text", "text/", "/plain", "text/plain;", "text/plain; =x",
		"text/plain; a=\"b", "text/plain; charset=ü", "*/*", "x/y; z=1; z=2"}
	exts := []string{".html", ".HTML", "html", ".png", ".json", ".txt", ".unknown", "", ".tar.gz", ".js", ".wasm"}
	kinds := []string{"text/html", "application/json", "image/png", "text/plain", "audio/mpeg", "x/y", ""}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("mime.%04d", i)
		switch i % 5 {
		case 0:
			out = append(out, fuzzCase{name, "fz_mt(" + xq(pick(r, types)) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_mime_ext(" + xq(pick(r, exts)) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_mime_exts(" + xq(pick(r, kinds)) + ")"})
		case 3:
			out = append(out, fuzzCase{name, "fz_hx(mime.detect_content_type(" + xbytes(randBytes(r)) + "))"})
		case 4:
			kind := pick(r, kinds)
			params := "{}"
			if r.Intn(2) == 0 {
				params = "{\"charset\": " + xq(pick(r, []string{"utf-8", "US-ASCII", "iso-8859-1", "ü", ""})) + "}"
			}
			out = append(out, fuzzCase{name, "fz_hx(mime.format_media_type(" + xq(kind) + ", " + params + "))"})
		}
	}
	return out
}

// genMultipart covers parse, parse_form, write and the content type over
// bodies both built by hand and written by write itself.
func genMultipart(r *rand.Rand, n int) []fuzzCase {
	boundaries := []string{"abc", "--abc", "", "a b", "----=_Part_1", "x", "é", "a\"b", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		boundary := pick(r, boundaries)
		b := randBytes(r)
		name := fmt.Sprintf("multipart.%04d", i)
		switch i % 5 {
		case 0:
			body := "--" + boundary + "\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\nhi\r\n--" + boundary + "--\r\n"
			out = append(out, fuzzCase{name, "fz_mp(" + xbytes([]byte(body)) + ", " + xq(boundary) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_mp(" + xbytes(b) + ", " + xq(boundary) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_mp_write(" + xq(pick(r, pool)) + ", " + xbytes(b) + ", " + xq(boundary) + ")"})
		case 3:
			out = append(out, fuzzCase{name, "fz_mp_form(" + xbytes(b) + ", " + xq(boundary) + ")"})
		case 4:
			out = append(out, fuzzCase{name, "fz_hx(multipart.form_data_content_type(" + xq(boundary) + "))"})
		}
	}
	return out
}

// genXML covers parse and encode (strict and loose) and the token count
// over documents, malformed documents and fragments.
func genXML(r *rand.Rand, n int) []fuzzCase {
	texts := []string{"<a/>", "<a></a>", "<a x=\"1\">t</a>", "<a><b/><c>d</c></a>",
		"<a x='1' y=\"2\"/>", "<a>&amp;&lt;&#65;&#x41;</a>", "<a><![CDATA[x<y]]></a>",
		"<?xml version=\"1.0\"?><a/>", "<!-- c --><a/>", "<a><!-- c --></a>",
		"<a xmlns=\"u\"><b xmlns=\"v\"/></a>", "<a>text</a>", "<a><b></a>", "<a>",
		"", "text", "<a/><b/>", " <a/> ", "<a x=\"1\" x=\"2\"/>", "<1a/>", "<a b/>",
		"<!DOCTYPE a><a/>", "<a>\u0001</a>", "<a>&bogus;</a>", "<a>é🚀</a>", "<a><b><c><d/></c></b></a>"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		s := pick(r, texts)
		if r.Intn(3) == 0 {
			s = "<r>" + s + "</r>"
		}
		name := fmt.Sprintf("xml.%04d", i)
		switch i % 4 {
		case 0:
			out = append(out, fuzzCase{name, "fz_xml(" + xq(s) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_xml_loose(" + xq(s) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_xml_n(" + xq(s) + ")"})
		case 3:
			out = append(out, fuzzCase{name, "fz_hx(xml.escape(" + xq(pick(r, pool)) + "))"})
		}
	}
	return out
}

// genSQL covers the in memory driver: a typed round trip through exec and
// query with every accessor, the optional accessors over NULL, the error
// paths (no rows, wrong type, no such column, syntax, unique violation,
// a transaction rolled back) and the query parameter list.
func genSQL(r *rand.Rand, n int) []fuzzCase {
	sqlTexts := []string{"", "a", "a'b", "a\"b", "é🚀", "a; b", "line\nbreak", strings.Repeat("x", 300)}
	queries := []string{"", "select", "select * from", "select * from nope", "select 1 from",
		"insert into t values", "select 'a'", "select 1 + 1", ";;;", "select 1/0"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("sql.%04d", i)
		switch i % 6 {
		case 0:
			out = append(out, fuzzCase{name, "fz_sql_rt(" + xq(pick(r, sqlTexts)) + ", " + fmt.Sprint(r.Int63()-r.Int63()) + ", " + xbytes(randBytes(r)) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_sql_norows()"})
		case 2:
			out = append(out, fuzzCase{name, "fz_sql_mismatch(" + xq(pick(r, sqlTexts)) + ")"})
		case 3:
			out = append(out, fuzzCase{name, "fz_sql_bad(" + xq(pick(r, queries)) + ")"})
		case 4:
			out = append(out, fuzzCase{name, "fz_sql_dup(" + fmt.Sprint(r.Intn(5)) + ")"})
		case 5:
			out = append(out, fuzzCase{name, "fz_sql_tx(" + fmt.Sprint(r.Intn(100)) + ")"})
		}
	}
	return out
}

// genHTTP covers cookie and Set-Cookie parsing and printing, the request
// line's query and headers, the form of a url-encoded body, and the
// text and json responses.
func genHTTP(r *rand.Rand, n int) []fuzzCase {
	cookieLines := []string{"a=b", "a=b; c=d", "a=\"b\"", "a=", "=b", "a", "", "; ", "a=b;",
		"a=b; c", "a b=c", "a=b; a=c", "$Version=1; a=b", "a=é", "a=\"b\\\"c\"",
		"a=b; Path=/; Domain=x; Max-Age=10; Secure; HttpOnly; SameSite=Lax"}
	urls := []string{"http://h/", "http://h/p?a=1", "http://h/p?a=1&a=2&b=%20",
		"http://h/p?a", "http://h/p?a=", "http://h/p?a=%zz", "http://h/p?", "http://h/p#f",
		"http://h/p?a=b+c", "http://h/p?a=%C3%A9", "http://h/p?A=1&a=2", "http://h/p?a=1;b=2"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("http.%04d", i)
		switch i % 6 {
		case 0:
			out = append(out, fuzzCase{name, "fz_cookie(" + xq(pick(r, cookieLines)) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_set_cookie(" + xq(pick(r, cookieLines)) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_req(" + xq(pick(r, urls)) + ")"})
		case 3:
			body := pick(r, []string{"a=1", "a=1&b=2", "a=%20", "a", "a=1&a=2", "a=%zz", "", "a=1;b=2", "a=b+c"})
			out = append(out, fuzzCase{name, "fz_form(" + xbytes([]byte(body)) + ")"})
		case 4:
			out = append(out, fuzzCase{name, "fz_text(" + fmt.Sprint(r.Intn(600)) + ", " + xq(pick(r, pool)) + ")"})
		case 5:
			out = append(out, fuzzCase{name, "fz_json_resp(" + xq(pick(r, pool)) + ")"})
		}
	}
	return out
}

// genTemplate covers Go's text/template over a json.Value: fields,
// pipelines, if, range, with, built in functions, the named templates
// and missingkey=error, plus parse and execution errors.
func genTemplate(r *rand.Rand, n int) []fuzzCase {
	srcs := []string{"", "plain", "{{.}}", "{{.a}}", "a {{.b}} c", "{{.a.b}}",
		"{{if .x}}y{{else}}z{{end}}", "{{if eq .a 1}}one{{else}}other{{end}}",
		"{{range .l}}{{.}},{{end}}", "{{range $i, $v := .l}}{{$i}}:{{$v}};{{end}}",
		"{{len .l}}", "{{index .l 1}}", "{{with .s}}{{.}}{{end}}",
		"{{.missing}}", "{{.a | printf \"%v\"}}", "{{html .b}}", "{{js .b}}",
		"{{urlquery .b}}", "{{printf \"%d-%s\" .a .b}}", "{{and .x .a}}", "{{or .y .b}}",
		"{{not .x}}", "{{.b}}{{.b}}", "{{/* c */}}x", "{{- .a -}}",
		"{{", "{{.a", "{{end}}", "{{nope}}", "{{if}}{{end}}", "{{range .x}}{{end}}",
		"{{template \"other\"}}", "{{.a.b.c}}", "{{index .l 9}}", "{{slice .b 1}}"}
	datas := []string{"{}", "null", "1", "\"s\"", "[]", "[1,2,3]",
		"{\"a\":1,\"b\":\"x\",\"l\":[1,2,3],\"x\":true}",
		"{\"a\":{\"b\":2},\"l\":[],\"x\":false}",
		"{\"a\":null,\"b\":\"<&>\\\"\",\"l\":[{\"a\":1}],\"x\":1}",
		"{\"a\":1.5,\"b\":\"é\",\"l\":[null,true]}", "{\"a\":1E2,\"l\":[1]}",
		"{\"a\":1,\"b\":\"x\",\"l\":[[1],[2]]}"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		src, data := pick(r, srcs), pick(r, datas)
		name := fmt.Sprintf("template.%04d", i)
		switch i % 3 {
		case 0:
			out = append(out, fuzzCase{name, "fz_tmpl(" + xq(src) + ", " + xq(data) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_tmpl_missing(" + xq(src) + ", " + xq(data) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_tmpl_named(" + xq(src) + ", " + xq(pick(r, srcs)) + ", " + xq(data) + ")"})
		}
	}
	return out
}

// goGCMSeal is Go's AES-GCM sealing, the fiducial the native decoder must
// accept in the aes.open cases.
func goGCMSeal(key, nonce, pt, aad []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return g.Seal(nil, nonce, pt, aad)
}

// genCrypto covers the AEADs and streams of the native TLS stack through
// internal.crypto_op: AES-GCM and ChaCha20-Poly1305 seal, open (with a
// round trip, garbage that must fail the tag, and, for AES, ciphertext
// sealed by Go's own crypto/aes), AES blocks and CBC, the ChaCha20 block,
// xor and Poly1305, SHA-3 and SHAKE, HKDF and X25519.
func genCrypto(r *rand.Rand, n int) []fuzzCase {
	keyLens := []int{16, 24, 32}
	pickKey := func(l int) []byte { return randBytesN(r, l) }
	out := make([]fuzzCase, 0, n)
	randLen := func(max int) []byte { return randBytesN(r, r.Intn(max+1)) }
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("cryptostream.%04d", i)
		switch i % 14 {
		case 0:
			key := pickKey(keyLens[r.Intn(len(keyLens))])
			out = append(out, fuzzCase{name, "fz_cop(\"aes.block\", " + xbytes(key) + ", " + xbytes(randBytesN(r, 16)) + ", b\"\", b\"\")"})
		case 1:
			key := pickKey(keyLens[r.Intn(len(keyLens))])
			out = append(out, fuzzCase{name, "fz_cop(\"aes.decrypt\", " + xbytes(key) + ", " + xbytes(randBytesN(r, 16)) + ", b\"\", b\"\")"})
		case 2:
			key := pickKey(keyLens[r.Intn(len(keyLens))])
			data := randBytesN(r, 16*(r.Intn(4)))
			out = append(out, fuzzCase{name, "fz_cop(\"aes.cbc_encrypt\", " + xbytes(key) + ", " + xbytes(randBytesN(r, 16)) + ", " + xbytes(data) + ", b\"\")"})
		case 3:
			key := pickKey(keyLens[r.Intn(len(keyLens))])
			data := randBytesN(r, 16*(r.Intn(4)))
			out = append(out, fuzzCase{name, "fz_cop(\"aes.cbc_decrypt\", " + xbytes(key) + ", " + xbytes(randBytesN(r, 16)) + ", " + xbytes(data) + ", b\"\")"})
		case 4:
			key := pickKey(keyLens[r.Intn(len(keyLens))])
			out = append(out, fuzzCase{name, "fz_seal_open(\"aes\", " + xbytes(key) + ", " + xbytes(randBytesN(r, 12)) + ", " + xbytes(randLen(70)) + ", " + xbytes(randLen(20)) + ")"})
		case 5:
			key := pickKey(keyLens[r.Intn(len(keyLens))])
			nonce := randBytesN(r, 12)
			pt, aad := randLen(70), randLen(20)
			out = append(out, fuzzCase{name, "fz_cop(\"aes.open\", " + xbytes(key) + ", " + xbytes(nonce) + ", " + xbytes(goGCMSeal(key, nonce, pt, aad)) + ", " + xbytes(aad) + ")"})
		case 6:
			out = append(out, fuzzCase{name, "fz_cop(\"chacha.block\", " + xbytes(pickKey(32)) + ", " + xbytes(randBytesN(r, 12)) + ", " + xbytes([]byte(fmt.Sprint(r.Intn(5)))) + ", b\"\")"})
		case 7:
			out = append(out, fuzzCase{name, "fz_cop(\"chacha.xor\", " + xbytes(pickKey(32)) + ", " + xbytes(randBytesN(r, 12)) + ", " + xbytes([]byte(fmt.Sprint(r.Intn(5)))) + ", " + xbytes(randLen(80)) + ")"})
		case 8:
			out = append(out, fuzzCase{name, "fz_cop(\"chacha.poly1305\", " + xbytes(pickKey(32)) + ", " + xbytes(randLen(100)) + ", b\"\", b\"\")"})
		case 9:
			out = append(out, fuzzCase{name, "fz_seal_open(\"chacha\", " + xbytes(pickKey(32)) + ", " + xbytes(randBytesN(r, 12)) + ", " + xbytes(randLen(70)) + ", " + xbytes(randLen(20)) + ")"})
		case 10:
			data := randLen(100)
			op := pick(r, []string{"sha3_256", "sha3_512"})
			out = append(out, fuzzCase{name, "fz_cop(\"sha3." + op + "\", " + xbytes(data) + ", b\"\", b\"\", b\"\")"})
		case 11:
			data := randLen(100)
			outlen := []byte(fmt.Sprint(r.Intn(70)))
			op := pick(r, []string{"sha3.shake128", "sha3.shake256"})
			out = append(out, fuzzCase{name, "fz_cop(\"" + op + "\", " + xbytes(data) + ", " + xbytes(outlen) + ", b\"\", b\"\")"})
		case 12:
			if r.Intn(2) == 0 {
				out = append(out, fuzzCase{name, "fz_cop(\"hkdf.extract256\", " + xbytes(randLen(32)) + ", " + xbytes(randLen(80)) + ", b\"\", b\"\")"})
			} else {
				out = append(out, fuzzCase{name, "fz_cop(\"hkdf.expand256\", " + xbytes(pickKey(32)) + ", " + xbytes(randLen(40)) + ", " + xbytes([]byte(fmt.Sprint(r.Intn(70)))) + ", b\"\" )"})
			}
		case 13:
			if r.Intn(2) == 0 {
				out = append(out, fuzzCase{name, "fz_cop(\"x25519.base\", " + xbytes(pickKey(32)) + ", b\"\", b\"\", b\"\")"})
			} else {
				out = append(out, fuzzCase{name, "fz_cop(\"x25519.mult\", " + xbytes(pickKey(32)) + ", " + xbytes(pickKey(32)) + ", b\"\", b\"\")"})
			}
		}
	}
	return out
}

// randBytesN is n seeded random bytes.
func randBytesN(r *rand.Rand, n int) []byte {
	b := make([]byte, n)
	r.Read(b)
	return b
}

// ---------------------------------------------------------------------------
// x509: real certificates, made deterministically here, then mutated.

// detReader is a deterministic io.Reader over a math/rand source, so key
// generation and signing give the same certificates on every run.
type detReader struct{ r *rand.Rand }

func (d detReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(d.r.Intn(256))
	}
	return len(p), nil
}

var fuzzCertHexesOnce []string

// preludes are declarations an area needs outside fn main (the x509
// certificate table).
var preludes []string

// fuzzCertHexes builds the certificate set once: a P-256 root, a P-256
// leaf with DNS, IP, email and URI names, a P-384 expired certificate, an
// Ed25519 not-yet-valid one, an RSA name-constrained CA, a P-256
// certificate with an unknown critical extension, a P-256 certificate
// with an empty subject and an RSA root.
func fuzzCertHexes() []string {
	if fuzzCertHexesOnce != nil {
		return fuzzCertHexesOnce
	}
	dr := detReader{rand.New(rand.NewSource(99))}
	nb := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	na := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	ca := func(cn string, serial int64) *x509.Certificate {
		return &x509.Certificate{
			SerialNumber:          big.NewInt(serial),
			Subject:               pkix.Name{CommonName: cn, Organization: []string{"Fuzz"}, Country: []string{"US"}},
			NotBefore:             nb,
			NotAfter:              na,
			IsCA:                  true,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
			SubjectKeyId:          []byte{1, 2, 3, 4},
		}
	}
	leaf := ca("fuzz leaf", 2)
	leaf.IsCA = false
	leaf.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	u, err := url.Parse("https://example.com/a?b=c")
	if err != nil {
		panic(err)
	}
	leaf.DNSNames = []string{"example.com", "*.example.com", "xn--bcher-kva.example"}
	leaf.IPAddresses = []net.IP{net.ParseIP("192.0.2.1"), net.ParseIP("2001:db8::1")}
	leaf.EmailAddresses = []string{"a@example.com"}
	leaf.URIs = []*url.URL{u}
	leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}

	expired := ca("fuzz expired", 3)
	expired.NotBefore = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	expired.NotAfter = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

	future := ca("fuzz future", 4)
	future.NotBefore = time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
	future.NotAfter = time.Date(2060, 1, 1, 0, 0, 0, 0, time.UTC)

	constrained := ca("fuzz constrained", 5)
	constrained.PermittedDNSDomainsCritical = true
	constrained.PermittedDNSDomains = []string{".example.com", "other.example"}
	constrained.ExcludedDNSDomains = []string{".bad.example"}
	constrained.PermittedIPRanges = []*net.IPNet{{IP: net.ParseIP("192.0.2.0").To4(), Mask: net.CIDRMask(24, 32)}}

	unknown := ca("fuzz unknown ext", 6)
	unknown.ExtraExtensions = []pkix.Extension{{
		Id:       asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1},
		Critical: true,
		Value:    []byte{5, 0},
	}}

	empty := ca("", 7)
	empty.Subject = pkix.Name{}
	empty.SubjectKeyId = nil

	type kp struct {
		priv any
		pub  any
	}
	mkKey := func(kind int) kp {
		switch kind {
		case 0:
			k, err := ecdsa.GenerateKey(elliptic.P256(), dr)
			if err != nil {
				panic(err)
			}
			return kp{k, &k.PublicKey}
		case 1:
			k, err := ecdsa.GenerateKey(elliptic.P384(), dr)
			if err != nil {
				panic(err)
			}
			return kp{k, &k.PublicKey}
		case 2:
			pub, priv, err := ed25519.GenerateKey(dr)
			if err != nil {
				panic(err)
			}
			return kp{priv, pub}
		default:
			k, err := rsa.GenerateKey(dr, 2048)
			if err != nil {
				panic(err)
			}
			return kp{k, &k.PublicKey}
		}
	}
	out := make([]string, 0, 8)
	for i, tm := range []*x509.Certificate{nil, leaf, expired, future, constrained, unknown, empty, nil} {
		var k kp
		switch i {
		case 0:
			tm, k = ca("fuzz root", 1), mkKey(0)
		case 2:
			k = mkKey(1)
		case 3:
			k = mkKey(2)
		case 4:
			k = mkKey(3)
		case 7:
			tm, k = ca("fuzz rsa root", 8), mkKey(3)
		default:
			k = mkKey(0)
		}
		der, err := x509.CreateCertificate(dr, tm, tm, k.pub, k.priv)
		if err != nil {
			panic(fmt.Sprintf("stdfuzz: creating fuzz certificate %d: %v", i, err))
		}
		out = append(out, fmt.Sprintf("%x", der))
	}
	fuzzCertHexesOnce = out
	return out
}

// certTable is the Xo const the x509 cases read, one certificate a line.
func certTable(hexes []string) string {
	var b strings.Builder
	b.WriteString("\n/// the certificates the cases mutate, in hex (made by the generator with crypto/x509 from a fixed seed)\nconst CERT_HEX: List[Str] = [\n")
	for _, h := range hexes {
		fmt.Fprintf(&b, "  %s,\n", xq(h))
	}
	b.WriteString("]\n")
	return b.String()
}

// genX509 covers the certificate parser through internal.crypto_op:
// x509.check on an unmodified certificate, on one with a changed nibble
// (outside the first byte, so the outer length stays sane) and on one cut
// short, and x509.signed_by over every pair of the set.
func genX509(r *rand.Rand, n int) []fuzzCase {
	hexes := fuzzCertHexes()
	preludes = append(preludes, certTable(hexes))
	out := []fuzzCase{{Name: "x509.0000", Expr: "fz_x509_plain(0)"}}
	for i := 1; i < n; i++ {
		name := fmt.Sprintf("x509.%04d", i)
		idx := r.Intn(len(hexes))
		hlen := len(hexes[idx])
		switch i % 4 {
		case 0:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_x509_plain(%d)", idx)})
		case 1:
			pos := 2 + r.Intn(hlen-2)
			nib := string("0123456789abcdef"[r.Intn(16)])
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_x509_mut(%d, %d, %q)", idx, pos, nib)})
		case 2:
			cut := 2 * (r.Intn(hlen/2) + 1)
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_x509_cut(%d, %d)", idx, cut)})
		default:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_x509_pair(%d, %d)", idx, r.Intn(len(hexes)))})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// ecc: ECDSA verify/ECDH on P-256 and P-384, and RSA verify, with keys and
// signatures made deterministically here.

type ecSet struct {
	curve string
	priv  []byte
	pub   []byte
	hash  []byte
	sig   []byte
	bad   []byte // sig with one byte flipped
	short []byte // sig cut in half
	pubB  []byte
	privB []byte
}

type rsaSet struct {
	priv   []byte
	pub    []byte
	hash   []byte
	sig    []byte
	sigBad []byte
	pss    []byte
}

var (
	ecOnce  []ecSet
	rsaOnce *rsaSet
)

func ecSets() []ecSet {
	if ecOnce != nil {
		return ecOnce
	}
	dr := detReader{rand.New(rand.NewSource(1234))}
	for _, c := range []struct {
		name  string
		curve elliptic.Curve
	}{
		{"p256", elliptic.P256()},
		{"p384", elliptic.P384()},
	} {
		size := (c.curve.Params().BitSize + 7) / 8
		key, err := ecdsa.GenerateKey(c.curve, dr)
		if err != nil {
			panic(err)
		}
		peer, err := ecdsa.GenerateKey(c.curve, dr)
		if err != nil {
			panic(err)
		}
		hash := randBytesN(dr.r, size)
		sig, err := ecdsa.SignASN1(dr, key, hash)
		if err != nil {
			panic(err)
		}
		bad := append([]byte(nil), sig...)
		bad[len(bad)/2] ^= 1
		ecOnce = append(ecOnce, ecSet{
			curve: c.name,
			priv:  key.D.FillBytes(make([]byte, size)),
			pub:   elliptic.Marshal(c.curve, key.X, key.Y),
			hash:  hash,
			sig:   sig,
			bad:   bad,
			short: sig[:len(sig)/2],
			pubB:  elliptic.Marshal(c.curve, peer.X, peer.Y),
			privB: peer.D.FillBytes(make([]byte, size)),
		})
	}
	return ecOnce
}

func rsaSetOf() *rsaSet {
	if rsaOnce != nil {
		return rsaOnce
	}
	dr := detReader{rand.New(rand.NewSource(4321))}
	key, err := rsa.GenerateKey(dr, 2048)
	if err != nil {
		panic(err)
	}
	hash := randBytesN(dr.r, 32)
	sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, hash)
	if err != nil {
		panic(err)
	}
	pss, err := rsa.SignPSS(dr, key, crypto.SHA256, hash, &rsa.PSSOptions{SaltLength: 32, Hash: crypto.SHA256})
	if err != nil {
		panic(err)
	}
	bad := append([]byte(nil), sig...)
	bad[len(bad)/2] ^= 1
	rsaOnce = &rsaSet{
		priv:   x509.MarshalPKCS1PrivateKey(key),
		pub:    x509.MarshalPKCS1PublicKey(&key.PublicKey),
		hash:   hash,
		sig:    sig,
		sigBad: bad,
		pss:    pss,
	}
	return rsaOnce
}

func eccTable() string {
	var b strings.Builder
	ecs := ecSets()
	b.WriteString("\n/// the ECDSA key material the ecc cases use, in hex (made by the generator with crypto/ecdsa from a fixed seed): priv, pub, hash, sig, a flipped sig, a short sig, a peer pub and peer priv, per curve p256 then p384\nconst EC_PRIV: List[Str] = [\n")
	for _, e := range ecs {
		fmt.Fprintf(&b, "  %s,\n", xq(fmt.Sprintf("%x", e.priv)))
	}
	b.WriteString("]\nconst EC_PUB: List[Str] = [\n")
	for _, e := range ecs {
		fmt.Fprintf(&b, "  %s,\n", xq(fmt.Sprintf("%x", e.pub)))
	}
	b.WriteString("]\nconst EC_HASH: List[Str] = [\n")
	for _, e := range ecs {
		fmt.Fprintf(&b, "  %s,\n", xq(fmt.Sprintf("%x", e.hash)))
	}
	b.WriteString("]\nconst EC_SIG: List[Str] = [\n")
	for _, e := range ecs {
		fmt.Fprintf(&b, "  %s,\n", xq(fmt.Sprintf("%x", e.sig)))
	}
	b.WriteString("]\nconst EC_SIG_BAD: List[Str] = [\n")
	for _, e := range ecs {
		fmt.Fprintf(&b, "  %s,\n", xq(fmt.Sprintf("%x", e.bad)))
	}
	b.WriteString("]\nconst EC_SIG_SHORT: List[Str] = [\n")
	for _, e := range ecs {
		fmt.Fprintf(&b, "  %s,\n", xq(fmt.Sprintf("%x", e.short)))
	}
	b.WriteString("]\nconst EC_PUB_B: List[Str] = [\n")
	for _, e := range ecs {
		fmt.Fprintf(&b, "  %s,\n", xq(fmt.Sprintf("%x", e.pubB)))
	}
	b.WriteString("]\nconst EC_PRIV_B: List[Str] = [\n")
	for _, e := range ecs {
		fmt.Fprintf(&b, "  %s,\n", xq(fmt.Sprintf("%x", e.privB)))
	}
	b.WriteString("]\n")
	r := rsaSetOf()
	fmt.Fprintf(&b, "\n/// the RSA-2048 key material the ecc cases use, in hex\nconst RSA_PRIV: Str = %s\nconst RSA_PUB: Str = %s\nconst RSA_HASH: Str = %s\nconst RSA_SIG: Str = %s\nconst RSA_SIG_BAD: Str = %s\nconst RSA_PSS: Str = %s\n",
		xq(fmt.Sprintf("%x", r.priv)), xq(fmt.Sprintf("%x", r.pub)), xq(fmt.Sprintf("%x", r.hash)),
		xq(fmt.Sprintf("%x", r.sig)), xq(fmt.Sprintf("%x", r.sigBad)), xq(fmt.Sprintf("%x", r.pss)))
	return b.String()
}

// genECC covers ECDSA verify (good, one flipped byte, half a signature),
// the public key of a scalar, ECDH both ways and with a bad peer point, on
// P-256 and P-384; and RSA PKCS #1 v1.5 and PSS verify with the same
// mutations, plus the native PKCS #1 signature against Go's.
func genECC(r *rand.Rand, n int) []fuzzCase {
	preludes = append(preludes, eccTable())
	ecs := ecSets()
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("ecc.%04d", i)
		k := r.Intn(len(ecs))
		switch i % 8 {
		case 0:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_ecv(%d, %d, 0)", k, k)})
		case 1:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_ecv(%d, %d, 1)", k, k)})
		case 2:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_ecv(%d, %d, 2)", k, k)})
		case 3:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_ecpub(%d)", k)})
		case 4:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_ecdh(%d)", k)})
		case 5:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_ecdh_bad(%d, %d)", k, r.Intn(40))})
		case 6:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_rsa(%d)", r.Intn(4))})
		case 7:
			out = append(out, fuzzCase{name, "fz_rsa_sign()"})
		}
	}
	return out
}

// genLimits puts inputs at the chunk and buffer edges: the empty input,
// one byte either side of 16, 32, 64, 256, 512, 1K, 2K, 4K, 8K, 16K,
// 32K, 64K, 128K and 256K, and 1 MiB, through the Str/Bytes operations
// that may special case a length (rune counts, search, replace, trim),
// then base64, base32, gzip, JSON, CSV, URL escaping and split, whose
// results must match on every backend. Every input is a short pattern
// repeated, so no case carries a large literal.
func genLimits(r *rand.Rand, n int) []fuzzCase {
	sizes := []int{0, 1, 2, 3, 7, 15, 16, 17, 31, 32, 33, 63, 64, 65, 127, 128, 129,
		255, 256, 257, 511, 512, 513, 1023, 1024, 1025, 2047, 4095, 4096, 4097,
		8191, 8192, 8193, 16383, 16384, 16385, 32767, 32768, 32769, 65535, 65536,
		65537, 131071, 131072, 262143, 262144, 1048575, 1048576}
	pats := []string{"a", "ab", "abc", "a b", "a,b", "a\n", "\\t", "é", "aé",
		"🚀", "a🚀", "é🚀", "\"", "\\\\"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("limits.%04d", i)
		kind := i % 12
		sz := sizes[r.Intn(len(sizes))]
		if kind == 10 || kind == 11 {
			// the CSV row and the escaped query stay small: both build
			// lists or multiply the text
			if sz > 65536 {
				sz = 65536
			}
		}
		if kind == 12 && sz > 65536 {
			sz = 65536
		}
		out = append(out, fuzzCase{name, fmt.Sprintf("fz_lim(%d, %s, %d)", kind, xq(pick(r, pats)), sz)})
	}
	return out
}
