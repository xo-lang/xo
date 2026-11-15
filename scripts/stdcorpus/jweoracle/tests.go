package main

// jweHelpers are the key helpers of the corpus and of the reverse program.
const jweHelpers = `
fn pem_der(text: Str) -> Bytes { crypto.pem_decode(text)[0].bytes }

fn sym(n: Int, flip: Bool) -> Bytes {
  var b = BytesBuilder.new()
  var i = 0
  while i < n {
    var v = (7 * i + n) & 255
    if flip && i == 0 {
      v = v ^ 1
    }
    b.write_byte(UInt8(v))
    i += 1
  }
  b.freeze()
}

/// The decrypting key of a name (as jweoracle's decKey).
fn dec_key(name: Str) -> jwt.JweKey {
  if name == "pw" {
    return jwt.JweKey.Password(password: Secret.new(PASSWORD))
  }
  if name == "pwx" {
    return jwt.JweKey.Password(password: Secret.new(PASSWORD + "!"))
  }
  if name.starts_with("symx") {
    return jwt.JweKey.Symmetric(key: Secret.new(sym(Int.parse(name[4..]).or_fault("size"), true)))
  }
  if name.starts_with("sym") {
    return jwt.JweKey.Symmetric(key: Secret.new(sym(Int.parse(name[3..]).or_fault("size"), false)))
  }
  for (n, priv, _pub) in KEYS {
    if n == name {
      return jwt.JweKey.Private(key: crypto.parse_private_key(pem_der(priv)).ok() ?? fault("private key"))
    }
  }
  fault("no key ${name}")
}

/// The encrypting key: public for RSA and EC.
fn enc_key(name: Str) -> jwt.JweKey {
  for (n, _priv, pubpem) in KEYS {
    if n == name {
      return jwt.JweKey.Public(key: crypto.parse_public_key(pem_der(pubpem)).ok() ?? fault("public key"))
    }
  }
  dec_key(name)
}

fn all_algs() -> List[jwt.KeyAlg] {
  var out: List[jwt.KeyAlg] = []
  for (a, _e, _k) in COMBOS {
    if !out.contains(a) {
      out.push(a)
    }
  }
  out
}

fn all_encs() -> List[jwt.Enc] {
  var out: List[jwt.Enc] = []
  for (_a, e, _k) in COMBOS {
    if !out.contains(e) {
      out.push(e)
    }
  }
  out
}

fn outcome(r: Result[Bytes, jwt.JwtErr]) -> Str {
  match r {
    Ok(p) => "ok ${p.to_hex()}"
    Err(_) => "err"
  }
}

`

const jweTests = `test "go-jose's tokens decrypt, or fail, as go-jose's Decrypt does" {
  let algs = all_algs()
  let encs = all_encs()
  var bad: List[Str] = []
  for (tok, key, want) in TOKENS {
    let got = outcome(jwt.decrypt(tok, dec_key(key), algs, encs))
    if got != want {
      bad.push("${key} ${tok[0..(if tok.len() < 60 { tok.len() } else { 60 })]}: ${got} want ${want}")
    }
  }
  expect(bad) == []
}

test "encrypt then decrypt gives the payload back for every combination" {
  let algs = all_algs()
  let encs = all_encs()
  var bad: List[Str] = []
  for (i, (a, e, k)) in COMBOS.indexed() {
    let payload = "payload ${i} ".repeat(i % 7).bytes()
    let opts = jwt.JweOptions{kid: Some("k${i}"), typ: if i % 2 == 0 { Some("JWT") } else { None }, deflate: i % 3 == 0, p2c: 1000}
    match jwt.encrypt(os.rand, a, e, enc_key(k), payload, opts) {
      Ok(tok) => {
        match jwt.decrypt(tok, dec_key(k), algs, encs) {
          Ok(p) => {
            if p != payload {
              bad.push("${i}: payload differs")
            }
          }
          Err(err) => bad.push("${i}: ${err.display()}")
        }
        let h = jwt.peek_jwe_header(tok)?
        if h.alg != a || h.enc != e || h.kid != Some("k${i}") || h.zip != (i % 3 == 0) {
          bad.push("${i}: header")
        }
        // a key or encryption outside the allowed lists
        if !(jwt.decrypt(tok, dec_key(k), [], encs) is Err(jwt.JwtErr.Alg(..))) || !(jwt.decrypt(tok, dec_key(k), algs, []) is Err(jwt.JwtErr.Alg(..))) {
          bad.push("${i}: allowed lists")
        }
      }
      Err(err) => bad.push("${i}: encrypt: ${err.display()}")
    }
  }
  expect(bad) == []
}

test "keys that do not fit are refused before any cryptography" {
  let p = b"x"
  expect(jwt.encrypt(os.rand, jwt.KeyAlg.A128KW, jwt.Enc.A128Gcm, dec_key("sym32"), p) is Err(jwt.JwtErr.Alg(..))) == true
  expect(jwt.encrypt(os.rand, jwt.KeyAlg.Dir, jwt.Enc.A256Gcm, dec_key("sym16"), p) is Err(jwt.JwtErr.Alg(..))) == true
  expect(jwt.encrypt(os.rand, jwt.KeyAlg.RsaOaep, jwt.Enc.A128Gcm, enc_key("p256"), p) is Err(jwt.JwtErr.Alg(..))) == true
  expect(jwt.encrypt(os.rand, jwt.KeyAlg.EcdhEs, jwt.Enc.A128Gcm, enc_key("rsa"), p) is Err(jwt.JwtErr.Alg(..))) == true
  expect(jwt.encrypt(os.rand, jwt.KeyAlg.Pbes2Hs256A128KW, jwt.Enc.A128Gcm, dec_key("sym16"), p) is Err(jwt.JwtErr.Alg(..))) == true
  expect(jwt.decrypt("a.b.c", dec_key("sym16"), all_algs(), all_encs()) is Err(jwt.JwtErr.Malformed(..))) == true
  expect(jwt.JwtErr.Decrypt.display()) == "jwt: error in cryptographic primitive"
}
`
