package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"hash"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The API this corpus covers, as std/crypto.xo declares it: aead_key,
// AeadKey.seal and open for Aead.AesGcm and Aead.ChaCha20Poly1305 (Go's
// cipher.AEAD), seal_random and open_random, and hkdf_extract, hkdf_expand
// and hkdf (Go's crypto/hkdf).
//
// The inputs: crypto/cipher's aesGCMTests rows with a 12 byte nonce and a 16
// byte tag (the sizes std/crypto takes), each checked against Go's
// cipher.NewGCM here; crypto/hkdf's hkdfTests, checked against Go's
// hkdf.Key; Wycheproof's aes_gcm_test.json and chacha20_poly1305_test.json
// groups with a 96 bit nonce and a 128 bit tag, valid and invalid alike
// (an invalid one must not open), and the valid rows of its
// hkdf_sha1/256/384/512_test.json (its invalid rows ask for more than 255
// blocks, a fault in std/crypto); and seeded keys, nonces, aad and lengths
// sealed by Go's AES-GCM. Go's tests are Copyright 2009 The Go Authors,
// BSD-style license; Wycheproof (testdata/wycheproof) is Apache 2.0.

type aeadRow struct {
	key, nonce, pt, aad, sealed []byte
	valid                       bool
	why                         string
}

type hkdfRow struct {
	hash                    string // the Hash variant
	secret, salt, info, out []byte
}

var hkdfHashes = map[string]func() hash.Hash{
	"Sha1": sha1.New, "Sha256": sha256.New, "Sha384": sha512.New384, "Sha512": sha512.New,
}

type wycheFile struct {
	TestGroups []struct {
		IvSize  int `json:"ivSize"`
		TagSize int `json:"tagSize"`
		Tests   []struct {
			TcID    int    `json:"tcId"`
			Comment string `json:"comment"`
			Key     string `json:"key"`
			Iv      string `json:"iv"`
			Aad     string `json:"aad"`
			Msg     string `json:"msg"`
			Ct      string `json:"ct"`
			Tag     string `json:"tag"`
			Ikm     string `json:"ikm"`
			Salt    string `json:"salt"`
			Info    string `json:"info"`
			Size    int    `json:"size"`
			Okm     string `json:"okm"`
			Result  string `json:"result"`
		} `json:"tests"`
	} `json:"testGroups"`
}

func wycheproof(name string) wycheFile {
	_, self, _, _ := runtime.Caller(0)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(self), "testdata", "wycheproof", name))
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: reading Wycheproof:", err)
		os.Exit(1)
	}
	var f wycheFile
	if err := json.Unmarshal(b, &f); err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: parsing Wycheproof", name+":", err)
		os.Exit(1)
	}
	return f
}

func unhex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: bad hex in a vector:", s)
		os.Exit(1)
	}
	return b
}

func goGCM(key []byte) cipher.AEAD {
	b, err := aes.NewCipher(key)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus:", err)
		os.Exit(1)
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus:", err)
		os.Exit(1)
	}
	return g
}

// wycheAead is the file's rows with a 96 bit nonce and a 128 bit tag; for
// AES-GCM, each valid row is checked against Go's GCM and each invalid one
// is checked not to open.
func wycheAead(name string, gcm bool) []aeadRow {
	var rows []aeadRow
	for _, g := range wycheproof(name).TestGroups {
		if g.IvSize != 96 || g.TagSize != 128 {
			continue
		}
		for _, t := range g.Tests {
			r := aeadRow{unhex(t.Key), unhex(t.Iv), unhex(t.Msg), unhex(t.Aad),
				append(unhex(t.Ct), unhex(t.Tag)...), t.Result != "invalid",
				fmt.Sprintf("wycheproof %d %s", t.TcID, t.Comment)}
			if gcm && len(r.key) != 16 && len(r.key) != 24 && len(r.key) != 32 {
				continue
			}
			if gcm {
				a := goGCM(r.key)
				if r.valid && !bytes.Equal(a.Seal(nil, r.nonce, r.pt, r.aad), r.sealed) {
					fmt.Fprintln(os.Stderr, "stdcorpus: Go's GCM disagrees with Wycheproof", t.TcID)
					os.Exit(1)
				}
				if _, err := a.Open(nil, r.nonce, r.sealed, r.aad); (err == nil) != r.valid {
					fmt.Fprintln(os.Stderr, "stdcorpus: Go's GCM open disagrees with Wycheproof", t.TcID)
					os.Exit(1)
				}
			}
			rows = append(rows, r)
		}
	}
	return rows
}

func aeadCorpus() string {
	var o strings.Builder
	o.WriteString(header("crypto/cipher's aesGCMTests and crypto/hkdf's hkdfTests, Wycheproof's AES-GCM, ChaCha20-Poly1305 and HKDF vectors, and seeded AES-GCM rows sealed by Go, against std/crypto's AeadKey and hkdf (Go's tests are Copyright 2009 The Go Authors, BSD-style license; Wycheproof is Apache 2.0)"))
	o.WriteString("\nuse std/crypto\n")

	// AES-GCM: Go's table, Wycheproof, then seeded rows.
	var gcm []aeadRow
	gcm = append(gcm, goGCMRows()...)
	gcm = append(gcm, wycheAead("aes_gcm_test.json", true)...)
	r := rand.New(rand.NewSource(13))
	for _, n := range []int{0, 1, 15, 16, 17, 31, 63, 64, 65, 127, 128, 129, 255, 256, 257, 511, 1000, 4096} {
		for _, kl := range []int{16, 24, 32} {
			key := make([]byte, kl)
			nonce := make([]byte, 12)
			pt := make([]byte, n)
			aad := make([]byte, r.Intn(40))
			r.Read(key)
			r.Read(nonce)
			r.Read(pt)
			r.Read(aad)
			gcm = append(gcm, aeadRow{key, nonce, pt, aad, goGCM(key).Seal(nil, nonce, pt, aad), true,
				fmt.Sprintf("seeded %d bytes, %d byte key", n, kl)})
		}
	}
	writeAeadRows(&o, "AES_GCM", "AES-GCM", gcm)
	writeAeadRows(&o, "CHACHA", "ChaCha20-Poly1305", wycheAead("chacha20_poly1305_test.json", false))

	// HKDF: Go's table, then Wycheproof's valid rows; every one checked
	// against Go's hkdf.Key.
	var hk []hkdfRow
	hk = append(hk, goHKDFRows()...)
	for _, h := range []string{"1", "256", "384", "512"} {
		name := "Sha" + h
		for _, g := range wycheproof("hkdf_sha" + h + "_test.json").TestGroups {
			for _, t := range g.Tests {
				if t.Result != "valid" {
					continue
				}
				hk = append(hk, hkdfRow{name, unhex(t.Ikm), unhex(t.Salt), unhex(t.Info), unhex(t.Okm)})
			}
		}
	}
	for _, x := range hk {
		got, err := hkdf.Key(hkdfHashes[x.hash], x.secret, x.salt, string(x.info), len(x.out))
		if err != nil || !bytes.Equal(got, x.out) {
			fmt.Fprintln(os.Stderr, "stdcorpus: Go's hkdf.Key disagrees with a vector of", x.hash)
			os.Exit(1)
		}
	}
	o.WriteString("\n/// (hash, secret, salt, info, Go's hkdf.Key output)\nconst HKDF_VECTORS: List[(Str, Bytes, Bytes, Bytes, Bytes)] = [\n")
	for _, x := range hk {
		fmt.Fprintf(&o, "  (%s, %s, %s, %s, %s),\n", xq(x.hash), xbytes(x.secret), xbytes(x.salt), xbytes(x.info), xbytes(x.out))
	}
	o.WriteString("]\n")

	o.WriteString(`
fn shown(bad: List[Str]) -> List[Str] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

fn check_aead(alg: crypto.Aead, rows: List[(Bytes, Bytes, Bytes, Bytes, Bytes, Bool, Str)]) -> List[Str] {
  var bad: List[Str] = []
  for (key, nonce, pt, aad, sealed, valid, why) in rows {
    let k = match crypto.aead_key(alg, key) {
      Ok(k) => k
      Err(e) => {
        bad.push("${why}: aead_key: ${e.display()}")
        continue
      }
    }
    if valid {
      let got = k.seal(nonce, pt, aad)
      if got != sealed {
        bad.push("${why}: seal ${got.to_hex()}, want ${sealed.to_hex()}")
      }
      match k.open(nonce, sealed, aad) {
        Ok(p) => if p != pt { bad.push("${why}: open gave other bytes") }
        Err(e) => bad.push("${why}: open: ${e.display()}")
      }
    } else {
      match k.open(nonce, sealed, aad) {
        Ok(_) => bad.push("${why}: an invalid message opened")
        Err(e) => if e.display() != "cipher: message authentication failed" { bad.push("${why}: ${e.display()}") }
      }
    }
  }
  bad
}

test "AES-GCM seals and opens as Go's crypto/cipher and Wycheproof" {
  expect(shown(check_aead(crypto.Aead.AesGcm, AES_GCM_VECTORS))) == []
}

test "ChaCha20-Poly1305 seals and opens as Wycheproof" {
  expect(shown(check_aead(crypto.Aead.ChaCha20Poly1305, CHACHA_VECTORS))) == []
}

test "a changed byte anywhere does not open, with Go's one error text" {
  let k = crypto.aead_key(crypto.Aead.AesGcm, b"0123456789abcdef").ok() ?? fault("key")
  let c = crypto.aead_key(crypto.Aead.ChaCha20Poly1305, b"0123456789abcdef0123456789abcdef").ok() ?? fault("key")
  let nonce = b"nonce 12 byt"
  for key in [k, c] {
    let sealed = key.seal(nonce, b"attack at dawn", b"header")
    var bad = 0
    for i in 0..sealed.len() {
      var b = BytesBuilder.with_capacity(sealed.len())
      b.write(sealed[0..i])
      b.write_byte(sealed[i] ^ 1)
      b.write(sealed[i + 1..sealed.len()])
      match key.open(nonce, b.freeze(), b"header") {
        Ok(_) => bad += 1
        Err(e) => if e.display() != "cipher: message authentication failed" { bad += 1 }
      }
    }
    expect(bad) == 0
    expect(key.open(nonce, sealed, b"other").is_err()) == true
    expect(key.open(nonce, sealed[0..15], b"header").is_err()) == true
    expect(key.open(b"other nonce!", sealed, b"header").is_err()) == true
    expect(key.open(nonce, sealed, b"header").ok()) == Some(b"attack at dawn")
  }
}

test "aead_key takes the key sizes Go's ciphers take" {
  for n in [16, 24, 32] {
    expect(crypto.aead_key(crypto.Aead.AesGcm, b"k".repeat(n)).is_ok()) == true
  }
  for n in [0, 15, 17, 31, 33, 64] {
    expect(crypto.aead_key(crypto.Aead.AesGcm, b"k".repeat(n)).err().map(fn(e) { e.display() })) == Some("crypto/aes: invalid key size ${n}")
  }
  expect(crypto.aead_key(crypto.Aead.ChaCha20Poly1305, b"k".repeat(32)).is_ok()) == true
  expect(crypto.aead_key(crypto.Aead.ChaCha20Poly1305, b"k".repeat(16)).err().map(fn(e) { e.display() })) == Some("chacha20poly1305: bad key length")
  expect(crypto.Aead.AesGcm.nonce_size()) == 12
  expect(crypto.Aead.ChaCha20Poly1305.overhead()) == 16
}

test "seal_random puts a fresh nonce first and open_random reads it" {
  for alg in [crypto.Aead.AesGcm, crypto.Aead.ChaCha20Poly1305] {
    let k = crypto.aead_key(alg, b"z".repeat(32)).ok() ?? fault("key")
    expect(k.alg()) == alg
    // never its key material, as a PrivateKey (std.md 11)
    expect(k.debug()) == "<aeadkey>"
    let a = k.seal_random(os.rand, b"cookie value", b"aad")
    let b = k.seal_random(os.rand, b"cookie value", b"aad")
    expect(a.len()) == 12 + 12 + 16
    expect(a == b) == false
    expect(k.open_random(a, b"aad").ok()) == Some(b"cookie value")
    expect(k.open(a[0..12], a[12..a.len()], b"aad").ok()) == Some(b"cookie value")
    expect(k.open_random(a, b"other").is_err()) == true
    expect(k.open_random(a[0..27], b"aad").is_err()) == true
  }
}

fn hash_named(s: Str) -> crypto.Hash {
  match s {
    "Sha1" => crypto.Hash.Sha1
    "Sha256" => crypto.Hash.Sha256
    "Sha384" => crypto.Hash.Sha384
    _ => crypto.Hash.Sha512
  }
}

test "hkdf is Go's hkdf.Key, extract then expand" {
  var bad: List[Str] = []
  for (h, secret, salt, info, want) in HKDF_VECTORS {
    let n = want.len()
    let got = crypto.hkdf(hash_named(h), secret, salt, info, n)
    if got != want {
      bad.push("${h} ${n}: ${got.to_hex()}")
    }
    let two = crypto.hkdf_expand(hash_named(h), crypto.hkdf_extract(hash_named(h), secret, salt), info, n)
    if two != want {
      bad.push("${h} ${n} in two steps: ${two.to_hex()}")
    }
  }
  expect(shown(bad)) == []
}

test "hkdf over every Hash, and an expand of 0 and of the most bytes" {
  for h in [crypto.Hash.Md5, crypto.Hash.Sha1, crypto.Hash.Sha224, crypto.Hash.Sha256, crypto.Hash.Sha384, crypto.Hash.Sha512, crypto.Hash.Sha3_256, crypto.Hash.Sha3_512] {
    let prk = crypto.hkdf_extract(h, b"secret")
    expect(prk.len()) == h.size()
    expect(crypto.hkdf_expand(h, prk, b"info", 0).len()) == 0
    let most = crypto.hkdf_expand(h, prk, b"info", 255 * h.size())
    expect(most.len()) == 255 * h.size()
    expect(crypto.hkdf_expand(h, prk, b"info", 10)) == most[0..10]
  }
}
`)
	return o.String()
}

// writeAeadRows writes rows as the const NAME_VECTORS.
func writeAeadRows(o *strings.Builder, name, what string, rows []aeadRow) {
	fmt.Fprintf(o, "\n/// %s: (key, nonce, plaintext, aad, sealed, valid, where it comes from)\nconst %s_VECTORS: List[(Bytes, Bytes, Bytes, Bytes, Bytes, Bool, Str)] = [\n", what, name)
	for _, r := range rows {
		fmt.Fprintf(o, "  (%s, %s, %s, %s, %s, %v, %s),\n", xbytes(r.key), xbytes(r.nonce), xbytes(r.pt), xbytes(r.aad),
			xbytes(r.sealed), r.valid, xq(r.why))
	}
	o.WriteString("]\n")
}

// goGCMRows is crypto/cipher's aesGCMTests with a 12 byte nonce and a 16
// byte tag, each sealed by Go's GCM here to the table's result.
func goGCMRows() []aeadRow {
	var rows []aeadRow
	for i, c := range goTestTable("crypto/cipher", "gcm_test.go", "aesGCMTests") {
		if len(c.Elts) < 5 {
			continue
		}
		var f [5][]byte
		ok := true
		for j := 0; j < 5; j++ {
			s, good := strconvString(c.Elts[j])
			b, err := hex.DecodeString(s)
			if !good || err != nil {
				ok = false
				break
			}
			f[j] = b
		}
		if !ok || len(f[1]) != 12 || len(f[4]) != len(f[2])+16 {
			continue
		}
		if !bytes.Equal(goGCM(f[0]).Seal(nil, f[1], f[2], f[3]), f[4]) {
			continue // a row of another tag size
		}
		rows = append(rows, aeadRow{f[0], f[1], f[2], f[3], f[4], true, fmt.Sprintf("Go aesGCMTests %d", i)})
	}
	if len(rows) < 10 {
		fmt.Fprintln(os.Stderr, "stdcorpus: Go's aesGCMTests gave", len(rows), "rows")
		os.Exit(1)
	}
	return rows
}

// goHKDFRows is crypto/hkdf's hkdfTests: (hash, master, salt, prk, info,
// out); a nil salt is empty.
func goHKDFRows() []hkdfRow {
	var rows []hkdfRow
	for _, c := range goTestTable("crypto/hkdf", "hkdf_test.go", "hkdfTests") {
		if len(c.Elts) < 6 {
			continue
		}
		var name string
		switch cryptoGoName(c.Elts[0]) {
		case "sha1":
			name = "Sha1"
		case "sha256":
			name = "Sha256"
		case "sha384":
			name = "Sha384"
		case "sha512":
			name = "Sha512"
		default:
			continue
		}
		var f [6][]byte
		for j := 1; j < 6; j++ {
			if id, isNil := c.Elts[j].(*ast.Ident); isNil && id.Name == "nil" {
				continue
			}
			b, ok := goBytes(c.Elts[j])
			if !ok {
				fmt.Fprintln(os.Stderr, "stdcorpus: an hkdfTests row stdcorpus cannot read")
				os.Exit(1)
			}
			f[j] = b
		}
		rows = append(rows, hkdfRow{name, f[1], f[2], f[4], f[5]})
	}
	if len(rows) < 5 {
		fmt.Fprintln(os.Stderr, "stdcorpus: Go's hkdfTests gave", len(rows), "rows")
		os.Exit(1)
	}
	return rows
}
