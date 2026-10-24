package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"hash"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The API this corpus covers, as std/crypto.xo declares it: digest (Go's
// md5.Sum, sha1.Sum, sha256.Sum224, sha256.Sum256, sha512.Sum384,
// sha512.Sum512, sha3.Sum256, sha3.Sum512), hmac over the same hashes (Go's
// crypto/hmac), a Hasher written in pieces against the same digests, equal
// (Go's subtle.ConstantTimeCompare), and ed25519_public_key, ed25519_sign and
// ed25519_verify (Go's crypto/ed25519).
//
// The inputs are Go's: the golden tables of crypto/md5, crypto/sha1,
// crypto/sha256 and crypto/sha512, the FIPS 198 and RFC 4231 rows of
// crypto/hmac's hmacTests, crypto/subtle's ConstantTimeCompare table, and
// crypto/ed25519's testdata/sign.input.gz (the reference implementation's 128
// vectors, the same file TestGolden reads) with TestMalleability's
// non-canonical signature. Inputs longer than a block are Go's own
// string-repeated rows rebuilt here, and the seeded rows take their
// expectations from Go's packages. Go's crypto and subtle tests are Copyright
// 2009 The Go Authors, BSD-style license.

// goTestTable returns the elements of the named composite literal table in a
// Go test file, from GOROOT's src.
func goTestTable(dir, file, name string) []*ast.CompositeLit {
	path := filepath.Join(runtime.GOROOT(), "src", dir, file)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: reading Go's crypto tests:", err)
		os.Exit(1)
	}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.VAR {
			continue
		}
		for _, s := range g.Specs {
			vs, ok := s.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || vs.Names[0].Name != name || len(vs.Values) != 1 {
				continue
			}
			lit, ok := vs.Values[0].(*ast.CompositeLit)
			if !ok {
				continue
			}
			var out []*ast.CompositeLit
			for _, e := range lit.Elts {
				if c, ok := e.(*ast.CompositeLit); ok {
					out = append(out, c)
				}
			}
			return out
		}
	}
	fmt.Fprintf(os.Stderr, "stdcorpus: Go's %s has no table %s\n", file, name)
	os.Exit(1)
	return nil
}

// goBytes is the bytes a []byte cell of one of Go's tables holds: a composite
// literal of small integers, or a conversion of a string literal.
func goBytes(e ast.Expr) ([]byte, bool) {
	switch v := e.(type) {
	case *ast.CompositeLit:
		var out []byte
		for _, el := range v.Elts {
			n, ok := strconvInt(el)
			if !ok {
				return nil, false
			}
			out = append(out, byte(n))
		}
		return out, true
	case *ast.CallExpr:
		if len(v.Args) == 1 {
			if s, ok := strconvString(v.Args[0]); ok {
				return []byte(s), true
			}
		}
	}
	return nil, false
}

// cryptoHash is one hash of Xo's Hash enum with Go's constructor.
type cryptoHash struct {
	enum string // Xo's crypto.Hash member
	key  string // its name here, and in the corpus's table names
	new  func() hash.Hash
}

var cryptoHashes = []cryptoHash{
	{"Md5", "md5", md5.New},
	{"Sha1", "sha1", sha1.New},
	{"Sha224", "sha224", sha256.New224},
	{"Sha256", "sha256", sha256.New},
	{"Sha384", "sha384", sha512.New384},
	{"Sha512", "sha512", sha512.New},
	{"Sha3_256", "sha3_256", func() hash.Hash { return sha3.New256() }},
	{"Sha3_512", "sha3_512", func() hash.Hash { return sha3.New512() }},
}

// cryptoDigest is Go's digest of in with h.
func cryptoDigest(h cryptoHash, in []byte) []byte {
	x := h.new()
	x.Write(in)
	return x.Sum(nil)
}

// cryptoGoName maps Go's constructor in a table cell ("sha1.New",
// "sha256.New224") to the hash it names, or "" for one Xo has no binding for.
func cryptoGoName(e ast.Expr) string {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	switch pkg.Name + "." + sel.Sel.Name {
	case "md5.New":
		return "md5"
	case "sha1.New":
		return "sha1"
	case "sha256.New224":
		return "sha224"
	case "sha256.New":
		return "sha256"
	case "sha512.New384":
		return "sha384"
	case "sha512.New":
		return "sha512"
	case "sha3.New256", "sha3.NewLegacyKeccak256":
		if sel.Sel.Name == "New256" {
			return "sha3_256"
		}
	}
	return ""
}

// cryptoGolden reads one of the digest packages' golden tables: rows whose in
// and out are string literals, whichever order the struct writes them in
// (decided by which way Go's digest of the one matches the other). Rows with a
// computed input (strings.Repeat) are skipped; the boundary lengths they cover
// arrive from cryptoSeededInputs.
func cryptoGolden(dir, table, want string) [][2][]byte {
	var h cryptoHash
	for _, c := range cryptoHashes {
		if c.key == want {
			h = c
		}
	}
	var out [][2][]byte
	rows := goTestTable(dir, filepath.Base(dir)+"_test.go", table)
	for _, c := range rows {
		if len(c.Elts) < 2 {
			continue
		}
		a, ok1 := strconvString(c.Elts[0])
		b, ok2 := strconvString(c.Elts[1])
		if !ok1 || !ok2 {
			continue
		}
		if hex.EncodeToString(cryptoDigest(h, []byte(b))) == a {
			out = append(out, [2][]byte{[]byte(b), cryptoDigest(h, []byte(b))})
		}
	}
	return out
}

// cryptoSeededInputs are the lengths that a block based hash branches on (the
// padding boundaries) and two long ones, with 'a' repeated.
func cryptoSeededInputs() [][]byte {
	var out [][]byte
	for _, n := range []int{0, 1, 55, 56, 57, 63, 64, 65, 111, 112, 113, 127, 128, 129, 200, 1000, 1000000} {
		out = append(out, bytes.Repeat([]byte("a"), n))
	}
	out = append(out, bytes.Repeat([]byte("1234567890"), 100))
	out = append(out, []byte("\x00\x01\x02\xff\xfe\x80 the quick brown fox 日本語"))
	return out
}

func cryptoCorpus() string {
	var o strings.Builder
	o.WriteString(header("md5, sha1, sha224, sha256, sha384, sha512, sha3_256 and sha3_512 digests, HMAC over the six classic hashes, a Hasher written in pieces, equal, and ed25519 public keys, signatures and verification, on the golden tables of Go's crypto/md5, crypto/sha1, crypto/sha256, crypto/sha512, crypto/hmac and crypto/subtle tests, on crypto/ed25519's testdata/sign.input.gz vectors and its TestMalleability case, and on seeded inputs at the block and padding boundaries, against std/crypto (Go's crypto tests are Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\n")

	// Digests.
	for _, h := range cryptoHashes {
		var rows [][2][]byte
		switch h.key {
		case "md5":
			rows = cryptoGolden("crypto/md5", "golden", h.key)
		case "sha1":
			rows = cryptoGolden("crypto/sha1", "golden", h.key)
		case "sha224":
			rows = cryptoGolden("crypto/sha256", "golden224", h.key)
		case "sha256":
			rows = cryptoGolden("crypto/sha256", "golden", h.key)
		case "sha384":
			rows = cryptoGolden("crypto/sha512", "golden384", h.key)
		case "sha512":
			rows = cryptoGolden("crypto/sha512", "golden512", h.key)
		}
		seen := map[string]bool{}
		for _, r := range rows {
			seen[string(r[0])] = true
		}
		for _, in := range cryptoSeededInputs() {
			if seen[string(in)] {
				continue
			}
			rows = append(rows, [2][]byte{in, cryptoDigest(h, in)})
		}
		name := strings.ToUpper(h.key) + "_DIGESTS"
		fmt.Fprintf(&o, "\n/// (input, Go's %s digest in hex)\nconst %s: List[(Bytes, Str)] = [\n", h.key, name)
		for _, r := range rows {
			fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(r[0]), xq(hex.EncodeToString(r[1])))
		}
		o.WriteString("]\n")
	}

	// HMAC: Go's hmacTests rows for each hash, then seeded keys and messages
	// of the block boundary lengths, keyed the same way.
	for _, h := range cryptoHashes[:6] {
		type macRow struct{ key, msg, mac []byte }
		var rows []macRow
		for _, c := range goTestTable("crypto/hmac", "hmac_test.go", "hmacTests") {
			if len(c.Elts) < 4 || cryptoGoName(c.Elts[0]) != h.key {
				continue
			}
			key, ok1 := goBytes(c.Elts[1])
			msg, ok2 := goBytes(c.Elts[2])
			if !ok1 || !ok2 {
				continue
			}
			m := hmac.New(h.new, key)
			m.Write(msg)
			rows = append(rows, macRow{key, msg, m.Sum(nil)})
		}
		for _, n := range []int{0, 1, 63, 64, 65, 128, 200, 1000} {
			key := bytes.Repeat([]byte("key"), 1+n/3)
			key = key[:n]
			msg := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog"), 1+n/40)
			msg = msg[:n]
			m := hmac.New(h.new, key)
			m.Write(msg)
			rows = append(rows, macRow{key, msg, m.Sum(nil)})
		}
		name := strings.ToUpper(h.key) + "_HMACS"
		fmt.Fprintf(&o, "\n/// (key, message, Go's %s HMAC in hex)\nconst %s: List[(Bytes, Bytes, Str)] = [\n", h.key, name)
		for _, r := range rows {
			fmt.Fprintf(&o, "  (%s, %s, %s),\n", xbytes(r.key), xbytes(r.msg), xq(hex.EncodeToString(r.mac)))
		}
		o.WriteString("]\n")
	}

	// equal: Go's ConstantTimeCompare table, then the near misses.
	type eqRow struct {
		a, b []byte
		eq   bool
	}
	var eqs []eqRow
	for _, c := range goTestTable("crypto/subtle", "constant_time_test.go", "testConstantTimeCompareData") {
		if len(c.Elts) < 3 {
			continue
		}
		a, ok1 := goBytes(c.Elts[0])
		b, ok2 := goBytes(c.Elts[1])
		n, ok3 := strconvInt(c.Elts[2])
		if ok1 && ok2 && ok3 {
			// Go's table writes the answer as 1 or 0; ConstantTimeCompare
			// answers the same, and the two must agree
			want := subtle.ConstantTimeCompare(a, b) == 1
			if (n == 1) != want {
				fmt.Fprintln(os.Stderr, "stdcorpus: Go's ConstantTimeCompare table and answer disagree")
				os.Exit(1)
			}
			eqs = append(eqs, eqRow{a, b, want})
		}
	}
	eqs = append(eqs,
		eqRow{[]byte("abc"), []byte("abc"), true},
		eqRow{[]byte("abc"), []byte("abd"), false},
		eqRow{[]byte("abc"), []byte("ab"), false},
		eqRow{[]byte("ab"), []byte("abc"), false},
		eqRow{nil, nil, true},
		eqRow{nil, []byte{}, true},
		eqRow{[]byte{0xff, 0x00}, []byte{0xff, 0x00}, true},
		eqRow{[]byte{0xff, 0x00}, []byte{0xff, 0x01}, false})
	o.WriteString("\n/// (a, b, whether Go's subtle.ConstantTimeCompare says 1)\nconst EQUAL_CASES: List[(Bytes, Bytes, Bool)] = [\n")
	for _, r := range eqs {
		fmt.Fprintf(&o, "  (%s, %s, %v),\n", xbytes(r.a), xbytes(r.b), r.eq)
	}
	o.WriteString("]\n")

	// ed25519: the reference implementation's vectors.
	type signRow struct {
		seed, msg, pub, sig []byte
	}
	var signs []signRow
	path := filepath.Join(runtime.GOROOT(), "src", "crypto", "ed25519", "testdata", "sign.input.gz")
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: reading Go's ed25519 vectors:", err)
		os.Exit(1)
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: reading Go's ed25519 vectors:", err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(zr)
	for sc.Scan() {
		parts := strings.Split(sc.Text(), ":")
		if len(parts) != 5 {
			continue
		}
		seed, _ := hex.DecodeString(parts[0])
		pub, _ := hex.DecodeString(parts[1])
		msg, _ := hex.DecodeString(parts[2])
		sig, _ := hex.DecodeString(parts[3])
		if len(seed) != 64 || len(pub) != 32 || len(sig) < 64 {
			continue
		}
		signs = append(signs, signRow{seed[:32], msg, pub, sig[:64]})
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: reading Go's ed25519 vectors:", err)
		os.Exit(1)
	}
	f.Close()
	if len(signs) != 128 {
		fmt.Fprintf(os.Stderr, "stdcorpus: Go's ed25519 vectors gave %d rows, want 128\n", len(signs))
		os.Exit(1)
	}
	o.WriteString("\n/// (seed, message, the public key of the seed, the signature of the message, all as the reference implementation's vectors give them)\nconst ED25519_SIGN_CASES: List[(Bytes, Bytes, Str, Str)] = [\n")
	for _, r := range signs {
		fmt.Fprintf(&o, "  (%s, %s, %s, %s),\n", xbytes(r.seed), xbytes(r.msg),
			xq(hex.EncodeToString(r.pub)), xq(hex.EncodeToString(r.sig)))
	}
	o.WriteString("]\n")

	// The signatures a verifier must refuse, each confirmed with Go's
	// Verify before it goes in. The first is Go's TestMalleability row: s at
	// least the group order for the same message, which RFC 8032 section
	// 5.1.7 requires a verifier to reject (adding the order to a valid s
	// makes a second signature for the same message).
	type rejectRow struct {
		pub, msg, sig []byte
		why           string
	}
	var rejects []rejectRow
	malPub := []byte{0x7d, 0x4d, 0x0e, 0x7f, 0x61, 0x53, 0xa6, 0x9b, 0x62, 0x42,
		0xb5, 0x22, 0xab, 0xbe, 0xe6, 0x85, 0xfd, 0xa4, 0x42, 0x0f, 0x88, 0x34,
		0xb1, 0x08, 0xc3, 0xbd, 0xae, 0x36, 0x9e, 0xf5, 0x49, 0xfa}
	malSig := []byte{0x7c, 0x38, 0xe0, 0x26, 0xf2, 0x9e, 0x14, 0xaa, 0xbd, 0x05,
		0x9a, 0x0f, 0x2d, 0xb8, 0xb0, 0xcd, 0x78, 0x30, 0x40, 0x60, 0x9a, 0x8b,
		0xe6, 0x84, 0xdb, 0x12, 0xf8, 0x2a, 0x27, 0x77, 0x4a, 0xb0, 0x67, 0x65,
		0x4b, 0xce, 0x38, 0x32, 0xc2, 0xd7, 0x6f, 0x8f, 0x6f, 0x5d, 0xaf, 0xc0,
		0x8d, 0x93, 0x39, 0xd4, 0xee, 0xf6, 0x76, 0x57, 0x33, 0x36, 0xa5, 0xc5,
		0x1e, 0xb6, 0xf9, 0x46, 0xb3, 0x1d}
	rejects = append(rejects, rejectRow{malPub, []byte{0x54, 0x65, 0x73, 0x74},
		malSig, "Go's TestMalleability row: s is not below the group order (RFC 8032 5.1.7)"})
	// A vector's signature with its last byte's low bit flipped, one with its
	// first byte changed, and the all-zero signature: all invalid.
	v := signs[0]
	flip := append([]byte(nil), v.sig...)
	flip[63] ^= 1
	rejects = append(rejects, rejectRow{v.pub, v.msg, flip, "a valid vector's signature with the last byte changed"})
	flipR := append([]byte(nil), v.sig...)
	flipR[0] ^= 0x01
	rejects = append(rejects, rejectRow{v.pub, v.msg, flipR, "a valid vector's signature with its first byte changed"})
	rejects = append(rejects, rejectRow{v.pub, v.msg, make([]byte, 64), "an all-zero signature"})
	rejects = append(rejects, rejectRow{make([]byte, 32), []byte{}, make([]byte, 64), "an all-zero key, message and signature"})
	o.WriteString("\n/// (public key, message, signature in hex, why a verifier must refuse it)\nconst ED25519_REFUSED: List[(Bytes, Bytes, Str, Str)] = [\n")
	for _, r := range rejects {
		if ed25519.Verify(ed25519.PublicKey(r.pub), r.msg, r.sig) {
			fmt.Fprintf(os.Stderr, "stdcorpus: Go accepts the signature meant for %q\n", r.why)
			os.Exit(1)
		}
	}
	// the same signature under a different message, checked with the real one
	other := rejectRow{v.pub, append(append([]byte(nil), v.msg...), '!'), v.sig,
		"a valid signature under one byte more of message"}
	if ed25519.Verify(ed25519.PublicKey(other.pub), other.msg, other.sig) {
		fmt.Fprintln(os.Stderr, "stdcorpus: Go accepts the signature meant for one byte more of message")
		os.Exit(1)
	}
	rejects = append(rejects, other)
	for _, r := range rejects {
		fmt.Fprintf(&o, "  (%s, %s, %s, %s),\n", xbytes(r.pub), xbytes(r.msg),
			xq(hex.EncodeToString(r.sig)), xq(r.why))
	}
	o.WriteString("]\n")

	// Tests.
	o.WriteString(`
fn show3(bad: List[(Str, Str, Str)]) -> List[(Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}
`)
	for _, h := range cryptoHashes {
		name := strings.ToUpper(h.key)
		fmt.Fprintf(&o, `
test "crypto.digest %s is Go's %s" {
  var bad: List[(Str, Str, Str)] = []
  for (input, want) in %s_DIGESTS {
    let got = crypto.digest(crypto.Hash.%s, input).to_hex()
    if got != want {
      bad.push(("${input.to_hex().len()} bytes", got, want))
    }
  }
  expect(show3(bad)) == []
}
`, h.key, h.key, name, h.enum)
	}
	for _, h := range cryptoHashes[:6] {
		name := strings.ToUpper(h.key)
		fmt.Fprintf(&o, `
test "crypto.hmac %s is Go's HMAC-%s" {
  var bad: List[(Str, Str, Str)] = []
  for (key, msg, want) in %s_HMACS {
    let got = crypto.hmac(crypto.Hash.%s, key, msg).to_hex()
    if got != want {
      bad.push(("key ${key.to_hex().len()} msg ${msg.to_hex().len()}", got, want))
    }
  }
  expect(show3(bad)) == []
}
`, h.key, strings.ToUpper(h.key), name, h.enum)
	}
	o.WriteString(`
test "a Hasher written in pieces gives the same digest as one call" {
  var bad: List[(Str, Str, Str)] = []
  for (input, want) in SHA256_DIGESTS {
    let half = input.len() / 2
    var x = crypto.hasher(crypto.Hash.Sha256)
    x.write(input[0..half])
    let mid = x.written()
    x.write(input[half..input.len()])
    let got = x.sum().to_hex()
    if got != want {
      bad.push(("split ${input.len()} bytes", got, want))
    }
    if mid != half {
      bad.push(("written ${input.len()} bytes", "${mid}", "${half}"))
    }
    if x.written() != input.len() {
      bad.push(("written after ${input.len()} bytes", "${x.written()}", "${input.len()}"))
    }
  }
  for (key, msg, want) in SHA256_HMACS {
    let half = msg.len() / 2
    var x = crypto.hmac_hasher(crypto.Hash.Sha256, key)
    x.write(msg[0..half])
    x.write(msg[half..msg.len()])
    let got = x.sum().to_hex()
    if got != want {
      bad.push(("hmac split ${msg.len()} bytes", got, want))
    }
  }
  expect(show3(bad)) == []
}

test "crypto.equal is Go's ConstantTimeCompare" {
  var bad: List[(Str, Str, Str)] = []
  for (a, b, want) in EQUAL_CASES {
    let got = crypto.equal(a, b)
    if got != want {
      bad.push(("${a.to_hex()} ${b.to_hex()}", "${got}", "${want}"))
    }
  }
  expect(show3(bad)) == []
}

test "ed25519 public keys and signatures are the reference vectors, and verify" {
  var bad: List[(Str, Str, Str)] = []
  var n = 0
  for (seed, msg, want_pub, want_sig) in ED25519_SIGN_CASES {
    let pk_hex = crypto.ed25519_public_key(seed).to_hex()
    let sig = crypto.ed25519_sign(seed, msg).to_hex()
    let sig2 = crypto.ed25519_sign(seed, msg).to_hex()
    if pk_hex != want_pub {
      bad.push(("public key ${n}", pk_hex, want_pub))
    }
    if sig != want_sig {
      bad.push(("signature ${n}", sig, want_sig))
    }
    if sig2 != sig {
      bad.push(("signature ${n} twice", sig2, sig))
    }
    let pk = Bytes.from_hex(want_pub) ?? b""
    let sg = Bytes.from_hex(want_sig) ?? b""
    if !crypto.ed25519_verify(pk, msg, sg) {
      bad.push(("verify ${n}", "false", "true"))
    }
    n += 1
  }
  if n != ED25519_SIGN_CASES.len() {
    bad.push(("rows", "${n}", "${ED25519_SIGN_CASES.len()}"))
  }
  expect(show3(bad)) == []
}

test "ed25519 refuses a key that is not 32 bytes or a signature that is not 64" {
  // Xo's contract, std/crypto.xo: false rather than a fault. Go's Verify
  // panics on these, so this is not a Go expectation.
  let seed = Bytes.from_hex("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60") ?? b""
  let pk = crypto.ed25519_public_key(seed)
  let msg = b"Test"
  let sig = crypto.ed25519_sign(seed, msg)
  var bad: List[(Str, Str, Str)] = []
  if crypto.ed25519_verify(pk[0..31], msg, sig) {
    bad.push(("a 31 byte key", "accepted", "refused"))
  }
  if crypto.ed25519_verify(pk + b"\x00", msg, sig) {
    bad.push(("a 33 byte key", "accepted", "refused"))
  }
  if crypto.ed25519_verify(pk, msg, sig[0..63]) {
    bad.push(("a 63 byte signature", "accepted", "refused"))
  }
  if crypto.ed25519_verify(pk, msg, sig + b"\x00") {
    bad.push(("a 65 byte signature", "accepted", "refused"))
  }
  if crypto.ed25519_verify(b"", msg, sig) {
    bad.push(("an empty key", "accepted", "refused"))
  }
  expect(show3(bad)) == []
}

test "ed25519 refuses malleable, non-canonical and malformed signatures" {
  var bad: List[(Str, Str, Str)] = []
  for (pk, msg, sig_hex, why) in ED25519_REFUSED {
    let sig = Bytes.from_hex(sig_hex) ?? b""
    if crypto.ed25519_verify(pk, msg, sig) {
      bad.push((why, "accepted", "refused"))
    }
  }
  expect(show3(bad)) == []
}
`)
	return o.String()
}
