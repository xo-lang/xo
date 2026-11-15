package main

// tests is the corpus's test code, after its tables.
const tests = `fn unhex(s: Str) -> Bytes { Bytes.from_hex(s) ?? fault("bad hex ${s}") }

fn msg_of(r: Result[Bytes, crypto.KeyErr]) -> Str {
  match r {
    Ok(b) => b.to_hex()
    Err(e) => e.msg
  }
}

/// A seeded Rand: the same keys and nonces on every backend.
struct Stream {
  st: Mutex[Int]
}

fn (s: Stream) int(lo: Int, hi: Int) -> Int uses rand { lo }
fn (s: Stream) float() -> Float uses rand { 0.5 }
fn (s: Stream) bool() -> Bool uses rand { false }
fn (s: Stream) shuffle[T](xs: List[T]) -> List[T] uses rand { xs }
fn (s: Stream) bytes(n: Int) -> Bytes uses rand {
  var out = BytesBuilder.new()
  for _ in 0..n {
    let v = s.st.with(fn(var x: Int) -> Int {
      x = x.wrapping_mul(6364136223846793005).wrapping_add(1442695040888963407)
      x.shr_logical(33) & 255
    })
    out.write_byte(UInt8(v))
  }
  out.freeze()
}

test "digest, hmac, and a Hasher in two writes are x/crypto's" {
  for (hi, msg, want, key, mac, cut) in SUMS {
    let h = HASHES[hi]
    let m = unhex(msg)
    expect(crypto.digest(h, m).to_hex()) == want
    expect(crypto.hmac(h, unhex(key), m).to_hex()) == mac
    var x = crypto.hasher(h)
    x.write(m[0..cut])
    x.write(m[cut..m.len()])
    expect(x.sum().to_hex()) == want
    expect(x.written()) == m.len()
    var y = crypto.hmac_hasher(h, unhex(key))
    y.write(m[0..cut])
    y.write(m[cut..m.len()])
    expect(y.sum().to_hex()) == mac
  }
}

test "size and block_size are Go's Size and BlockSize" {
  for i in 0..HASHES.len() {
    let (size, block) = SIZES[i]
    expect(HASHES[i].size()) == size
    expect(crypto.hasher(HASHES[i]).size()) == size
    expect(crypto.hasher(HASHES[i]).block_size()) == block
    expect(crypto.hmac_hasher(HASHES[i], b"k").block_size()) == block
  }
}

test "marshal_binary is Go's MarshalBinary, and unmarshal_binary resumes it" {
  for (hi, msg, n, want) in STATES {
    let h = HASHES[hi]
    let m = unhex(msg)
    var x = crypto.hasher(h)
    x.write(m[0..n])
    expect((x.marshal_binary() ?? e => fault(e.msg)).to_hex()) == want
    var y = crypto.hasher(h).unmarshal_binary(unhex(want)) ?? e => fault(e.msg)
    // BLAKE2b's state holds its length; Keccak's, as SHA-3's, none
    expect(y.written()) == (if hi < 3 { n } else { 0 })
    y.write(m[n..m.len()])
    expect(y.sum()) == crypto.digest(h, m)
  }
}

test "unmarshal_binary's errors are Go's" {
  for (hi, st, want) in BAD_STATES {
    let got = match crypto.hasher(HASHES[hi]).unmarshal_binary(unhex(st)) {
      Ok(_) => ""
      Err(e) => e.msg
    }
    expect(got) == want
  }
}

test "blake2b_hasher is x/crypto's blake2b.New(size, key)" {
  for (size, key, msg, want) in KEYED {
    var x = crypto.blake2b_hasher(size, unhex(key)) ?? e => fault(e.msg)
    x.write(unhex(msg))
    expect(x.sum().to_hex()) == want
    expect(x.size()) == size
    expect(x.block_size()) == 128
    x.reset()
    x.write(unhex(msg))
    expect(x.sum().to_hex()) == want
    if key.is_empty() {
      let st = x.marshal_binary() ?? e => fault(e.msg)
      var y = crypto.blake2b_hasher(size) ?? e => fault(e.msg)
      y = y.unmarshal_binary(st) ?? e => fault(e.msg)
      expect(y.sum().to_hex()) == want
    } else {
      expect(x.marshal_binary().err().map(fn(e) { e.msg })) == Some(MAC_STATE_ERR)
    }
  }
  for (size, kl, want) in KEYED_ERRS {
    let got = match crypto.blake2b_hasher(size, b"k".repeat(kl)) {
      Ok(_) => ""
      Err(e) => e.msg
    }
    expect(got) == want
  }
}

test "blake2b_xof is x/crypto's blake2b.NewXOF, and a copy is its Clone" {
  for (size, key, msg, reads, want) in XOFS {
    var x = crypto.blake2b_xof(size, unhex(key)) ?? e => fault(e.msg)
    x.write(unhex(msg))
    var c = x
    for i in 0..reads.len() {
      expect(msg_of(x.read(reads[i]))) == want[i]
    }
    // the copy reads the same stream on its own
    expect(msg_of(c.read(reads[0]))) == want[0]
    x.reset()
    x.write(unhex(msg))
    expect(msg_of(x.read(reads[0]))) == want[0]
  }
  expect(crypto.blake2b_xof(10, b"k".repeat(65)).err().map(fn(e) { e.msg })) == Some(XOF_KEY_ERR)
  expect(crypto.blake2b_xof(4294967295).err().map(fn(e) { e.msg })) == Some(XOF_LENGTH_ERR)
  expect(crypto.BLAKE2B_OUTPUT_LENGTH_UNKNOWN) == 0
}

test "hkdf and pbkdf2 over BLAKE2b and Keccak are Go's" {
  for (hi, secret, salt, info, n, want) in HKDFS {
    expect(crypto.hkdf(HASHES[hi], unhex(secret), unhex(salt), unhex(info), n).to_hex()) == want
  }
  for (hi, pw, salt, it, n, want) in PBKDF2S {
    expect((crypto.pbkdf2(HASHES[hi], unhex(pw), unhex(salt), it, n) ?? e => fault(e.msg)).to_hex()) == want
  }
}

test "XChaCha20Poly1305 seals and opens as x/crypto's NewX" {
  for (key, nonce, pt, aad, want) in XCHACHA {
    let k = crypto.aead_key(crypto.Aead.XChaCha20Poly1305, unhex(key)) ?? e => fault(e.msg)
    expect(k.seal(unhex(nonce), unhex(pt), unhex(aad)).to_hex()) == want
    let sealed = unhex(want)
    expect((k.open(unhex(nonce), sealed, unhex(aad)) ?? e => fault(e.msg)).to_hex()) == pt
    let last = Int(sealed[sealed.len() - 1])
    let bad = sealed[0..sealed.len() - 1] + (if last == 0 { b"\x01" } else { b"\x00" })
    // std's one AEAD error for every message that does not open (x/crypto
    // says chacha20poly1305: where Go's cipher says cipher:)
    expect(k.open(unhex(nonce), bad, unhex(aad)).err().map(fn(e) { e.msg })) == Some("cipher: message authentication failed")
  }
  let (ns, ov) = XCHACHA_SIZES
  expect(crypto.Aead.XChaCha20Poly1305.nonce_size()) == ns
  expect(crypto.Aead.XChaCha20Poly1305.overhead()) == ov
  expect(crypto.aead_key(crypto.Aead.XChaCha20Poly1305, b"k".repeat(16)).err().map(fn(e) { e.msg })) == Some(XCHACHA_KEY_ERR)
  let k = crypto.aead_key(crypto.Aead.XChaCha20Poly1305, b"k".repeat(32)) ?? e => fault(e.msg)
  expect(k.nonce_size()) == ns
  let sealed = k.seal_random(Stream{st: Mutex[Int].new(5)}, b"hello", b"aad")
  expect(sealed.len()) == ns + 5 + ov
  expect(k.open_random(sealed, b"aad").ok()) == Some(b"hello")
}

test "RSA PKCS #1 v1.5 refuses BLAKE2b and Keccak (Go's SignPKCS1v15 has no DigestInfo for them), PSS takes BLAKE2b and OAEP both (Go's EncryptOAEP takes a hash.Hash); ECDSA signs with them" {
  let r = Stream{st: Mutex[Int].new(777)}
  let k = crypto.generate_key(r, crypto.KeyAlg.Rsa, 1024) ?? e => fault(e.msg)
  let p = k.public_key()
  for h in HASHES {
    let dg = crypto.digest(h, b"msg")
    expect(crypto.sign(r, k, h, b"msg").err().map(fn(e) { e.msg })) == Some(RSA_UNSUPPORTED)
    expect(crypto.sign_pkcs1v15_digest(k, Some(h), dg).err().map(fn(e) { e.msg })) == Some(RSA_UNSUPPORTED)
    expect(crypto.verify(p, h, b"msg", b"sig")) == false
    expect(crypto.verify_pkcs1v15_digest(p, Some(h), dg, b"sig")) == false
    let keccak = h == crypto.Hash.Keccak256 || h == crypto.Hash.Keccak512
    // a 1024-bit key holds PSS and OAEP of a hash up to 48 bytes
    let fits = 2 * h.size() + 2 <= 128
    if keccak {
      // no crypto.Hash for Go's SignPSS
      expect(crypto.sign_pss(r, k, h, b"msg").err().map(fn(e) { e.msg })) == Some(RSA_UNSUPPORTED)
      expect(crypto.sign_pss_digest(r, k, h, dg).err().map(fn(e) { e.msg })) == Some(RSA_UNSUPPORTED)
      expect(crypto.verify_pss(p, h, b"msg", b"sig")) == false
      expect(crypto.verify_pss_digest(p, h, dg, b"sig")) == false
    } else if fits {
      let sig = crypto.sign_pss(r, k, h, b"msg") ?? e => fault(e.msg)
      expect(crypto.verify_pss(p, h, b"msg", sig)) == true
      expect(crypto.verify_pss(p, h, b"msg!", sig)) == false
      let ds = crypto.sign_pss_digest(r, k, h, dg) ?? e => fault(e.msg)
      expect(crypto.verify_pss_digest(p, h, dg, ds)) == true
    } else {
      expect(crypto.sign_pss(r, k, h, b"msg").err().map(fn(e) { e.msg })) == Some(RSA_TOO_LONG)
    }
    if fits {
      let c = crypto.encrypt_oaep(r, p, h, b"msg", b"lbl") ?? e => fault(e.msg)
      expect(crypto.decrypt_oaep(k, h, c, b"lbl").ok()) == Some(b"msg")
      expect(crypto.decrypt_oaep(k, h, c, b"").err().map(fn(e) { e.msg })) == Some(RSA_DECRYPTION)
    } else {
      expect(crypto.encrypt_oaep(r, p, h, b"msg").err().map(fn(e) { e.msg })) == Some(RSA_TOO_LONG)
    }
    // an MGF1 hash apart from the OAEP hash is a crypto.Hash in Go's
    // OAEPOptions, which Keccak has none of
    if keccak {
      expect(crypto.encrypt_oaep(r, p, crypto.Hash.Sha256, b"msg", b"", Some(h)).err().map(fn(e) { e.msg })) == Some(RSA_UNSUPPORTED)
      expect(crypto.decrypt_oaep(k, crypto.Hash.Sha256, b"x", b"", Some(h)).err().map(fn(e) { e.msg })) == Some(RSA_UNSUPPORTED)
    } else {
      let c = crypto.encrypt_oaep(r, p, crypto.Hash.Sha256, b"msg", b"", Some(h)) ?? e => fault(e.msg)
      expect(crypto.decrypt_oaep(k, crypto.Hash.Sha256, c, b"", Some(h)).ok()) == Some(b"msg")
    }
  }
  let e = crypto.generate_key(r, crypto.KeyAlg.EcdsaP256) ?? e => fault(e.msg)
  for h in HASHES {
    let sig = crypto.sign(r, e, h, b"msg") ?? e => fault(e.msg)
    expect(crypto.verify(e.public_key(), h, b"msg", sig)) == true
    expect(crypto.verify(e.public_key(), h, b"msg!", sig)) == false
  }
}
`
