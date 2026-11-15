package main

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hpke"
	"crypto/mlkem"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	mrand "math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// std/hpke against Go's crypto/hpke: the base mode vectors of its
// testdata/rfc9180.json and testdata/hpke-pq.json, run as hpke_test.go's
// TestVectors runs them (the ephemeral randomness given by a Rand that
// answers fixed bytes); messages Go sealed, opened; DeriveKeyPair of
// seeded inputs; and Go's errors for bad identifiers, keys, encapsulated
// keys, ciphertexts and export lengths. Go's tests and test data are
// Copyright 2024 The Go Authors, BSD-style license.

var hpkeKEMs = []uint16{0x0010, 0x0011, 0x0012, 0x0020, 0x0041, 0x0042, 0x647a, 0x0050, 0x0051}
var hpkeKDFs = []uint16{0x0001, 0x0002, 0x0003, 0x0010, 0x0011}
var hpkeAEADs = []uint16{0x0001, 0x0002, 0x0003, 0xffff}

type hpkeVector struct {
	Mode           uint16                                `json:"mode"`
	KEM            uint16                                `json:"kem_id"`
	KDF            uint16                                `json:"kdf_id"`
	AEAD           uint16                                `json:"aead_id"`
	Info           string                                `json:"info"`
	IkmE           string                                `json:"ikmE"`
	IkmR           string                                `json:"ikmR"`
	SkRm           string                                `json:"skRm"`
	PkRm           string                                `json:"pkRm"`
	Enc            string                                `json:"enc"`
	Encryptions    []struct{ Aad, Ct, Nonce, Pt string } `json:"encryptions"`
	Exports        []hpkeExport                          `json:"exports"`
	AccEncryptions string                                `json:"encryptions_accumulated"`
	AccExports     string                                `json:"exports_accumulated"`
}

type hpkeExport struct {
	Context string `json:"exporter_context"`
	L       int    `json:"L"`
	Value   string `json:"exported_value"`
}

func hpkeMust[T any](v T, err error) T {
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus hpke:", err)
		os.Exit(1)
	}
	return v
}

func hpkeHex(s string) []byte { return hpkeMust(hex.DecodeString(s)) }

func hpkeErrText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// hpkeRand is the bytes std/hpke's encapsulation reads for a vector's
// ikmE: DHKEM's ephemeral key from DeriveKeyPair(ikmE) as
// ecdh_generate_key reads it (P-521's scalar shifted left 7 bits, as the
// sampler shifts its candidate right); the ML-KEM message, or a hybrid's
// message then its curve key, as Go's test gives them (ikmE itself).
func hpkeRand(kemID uint16, ikmE []byte) []byte {
	switch kemID {
	case 0x0010, 0x0011, 0x0012, 0x0020:
		kem := hpkeMust(hpke.NewKEM(kemID))
		sk := hpkeMust(hpkeMust(kem.DeriveKeyPair(ikmE)).Bytes())
		if kemID == 0x0012 {
			v := new(big.Int).Lsh(new(big.Int).SetBytes(sk), 7)
			return v.FillBytes(make([]byte, 66))
		}
		return sk
	case 0x0050:
		return ikmE[:64]
	}
	return ikmE
}

func hpkeVectors(name string) []hpkeVector {
	path := filepath.Join(runtime.GOROOT(), "src", "crypto", "hpke", "testdata", name)
	b := hpkeMust(os.ReadFile(path))
	var vs []hpkeVector
	hpkeMust(0, json.Unmarshal(b, &vs))
	var out []hpkeVector
	for _, v := range vs {
		if v.Mode != 0 || v.KEM == 0x0021 || v.KEM == 0x0040 || v.KDF == 0x0012 || v.KDF == 0x0013 {
			continue
		}
		out = append(out, v)
	}
	return out
}

func hpkeCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/hpke and its testdata/rfc9180.json and testdata/hpke-pq.json (Copyright 2024 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\nuse std/hpke\n")

	// The vectors.
	o.WriteString("\n/// (kem, kdf, aead, info, the Rand's bytes, pkRm, skRm, ikmR, enc, encryptions as (aad, pt, ct), exports as (context, L, value), accumulated encryptions, accumulated exports)\nconst VECTORS: List[(Int, Int, Int, Bytes, Bytes, Bytes, Bytes, Bytes, Bytes, List[(Bytes, Bytes, Bytes)], List[(Bytes, Int, Bytes)], Str, Str)] = [\n")
	n := 0
	for _, f := range []string{"rfc9180.json", "hpke-pq.json"} {
		for _, v := range hpkeVectors(f) {
			n++
			var encs, exps []string
			for _, e := range v.Encryptions {
				encs = append(encs, fmt.Sprintf("(%s, %s, %s)", xbytes(hpkeHex(e.Aad)), xbytes(hpkeHex(e.Pt)), xbytes(hpkeHex(e.Ct))))
			}
			for _, e := range v.Exports {
				exps = append(exps, fmt.Sprintf("(%s, %d, %s)", xbytes(hpkeHex(e.Context)), e.L, xbytes(hpkeHex(e.Value))))
			}
			fmt.Fprintf(&o, "  (%d, %d, %d, %s, %s, %s, %s, %s, %s, [%s], [%s], %s, %s),\n", v.KEM, v.KDF, v.AEAD,
				xbytes(hpkeHex(v.Info)), xbytes(hpkeRand(v.KEM, hpkeHex(v.IkmE))), xbytes(hpkeHex(v.PkRm)), xbytes(hpkeHex(v.SkRm)),
				xbytes(hpkeHex(v.IkmR)), xbytes(hpkeHex(v.Enc)), strings.Join(encs, ", "), strings.Join(exps, ", "), xq(v.AccEncryptions), xq(v.AccExports))
		}
	}
	o.WriteString("]\n")
	if n < 30 {
		fmt.Fprintln(os.Stderr, "stdcorpus hpke: too few vectors", n)
		os.Exit(1)
	}

	// DeriveKeyPair of seeded inputs, and messages Go sealed to that key.
	pins := loadHpkePins()
	defer pins.save()
	r := mrand.New(mrand.NewSource(646))
	o.WriteString("\n/// (kem, ikm, Go's private key bytes, public key bytes)\nconst DERIVED: List[(Int, Bytes, Bytes, Bytes)] = [\n")
	for _, id := range hpkeKEMs {
		kem := hpkeMust(hpke.NewKEM(id))
		for _, l := range []int{0, 1, 32, 64, 100} {
			ikm := make([]byte, l)
			r.Read(ikm)
			k := hpkeMust(kem.DeriveKeyPair(ikm))
			fmt.Fprintf(&o, "  (%d, %s, %s, %s),\n", id, xbytes(ikm), xbytes(hpkeMust(k.Bytes())), xbytes(k.PublicKey().Bytes()))
		}
	}
	o.WriteString("]\n")

	o.WriteString("\n/// (kem, ikm of the recipient's key, kdf, aead, info, message Go sealed, its plaintext or Go's error)\nconst SEALED: List[(Int, Bytes, Int, Int, Bytes, Bytes, Str)] = [\n")
	for i, id := range hpkeKEMs {
		kem := hpkeMust(hpke.NewKEM(id))
		ikm := []byte(fmt.Sprintf("recipient %d", i))
		k := hpkeMust(kem.DeriveKeyPair(ikm))
		for j, kdfID := range hpkeKDFs {
			if id >= 0x0041 && j != i%len(hpkeKDFs) {
				// the ML-KEM KEMs' messages are long: one KDF each
				continue
			}
			for _, aeadID := range hpkeAEADs[:3] {
				kdf := hpkeMust(hpke.NewKDF(kdfID))
				aead := hpkeMust(hpke.NewAEAD(aeadID))
				info := []byte(fmt.Sprintf("info %d", j))
				pt := make([]byte, r.Intn(80))
				r.Read(pt)
				c := pins.get(fmt.Sprintf("seal %d %d %d %d %x", id, kdfID, aeadID, j, pt), func() []byte {
					return hpkeMust(hpke.Seal(k.PublicKey(), kdf, aead, info, pt))
				}, func(c []byte) bool {
					p, err := hpke.Open(k, kdf, aead, info, c)
					return err == nil && bytes.Equal(p, pt)
				})
				// the message as sealed, one with a flipped byte (an AEAD
				// error), and one cut short
				variants := [][]byte{c, append([]byte(nil), c...), c[:len(c)-17]}
				variants[1][len(c)-1] ^= 0x80
				for _, m := range variants {
					p, err := hpke.Open(k, kdf, aead, info, m)
					want := hex.EncodeToString(p)
					if err != nil {
						want = "err " + err.Error()
					}
					fmt.Fprintf(&o, "  (%d, %s, %d, %d, %s, %s, %s),\n", id, xbytes(ikm), kdfID, aeadID, xbytes(info), xbytes(m), xq(want))
				}
			}
		}
	}
	o.WriteString("]\n")

	// Errors.
	o.WriteString("\n/// (what, Go's result: \"ok\" or its error)\nconst ERRORS: List[(Str, Str)] = [\n")
	add := func(what string, err error) {
		want := "ok"
		if err != nil {
			want = err.Error()
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(what), xq(want))
	}
	for _, id := range []uint16{0, 0x0013, 0x0021, 0x0040, 0x0041, 0xffff} {
		_, err := hpke.NewKEM(id)
		add(fmt.Sprintf("kem %d", id), err)
	}
	for _, id := range []uint16{0, 0x0004, 0x0012, 0x0013, 0x0011, 0xffff} {
		_, err := hpke.NewKDF(id)
		add(fmt.Sprintf("kdf %d", id), err)
	}
	for _, id := range []uint16{0, 0x0003, 0x0004, 0xfffe, 0xffff} {
		_, err := hpke.NewAEAD(id)
		add(fmt.Sprintf("aead %d", id), err)
	}
	for i, id := range hpkeKEMs {
		kem := hpkeMust(hpke.NewKEM(id))
		k := hpkeMust(kem.DeriveKeyPair([]byte(fmt.Sprintf("recipient %d", i))))
		pub := k.PublicKey().Bytes()
		priv := hpkeMust(k.Bytes())
		for _, b := range [][]byte{nil, pub[:len(pub)-1], append(append([]byte(nil), pub...), 0), hpkeFlip(pub, 0), hpkeFlip(pub, len(pub)-1), make([]byte, len(pub))} {
			_, err := kem.NewPublicKey(b)
			add(fmt.Sprintf("public %d %s", id, hex.EncodeToString(b)), err)
		}
		for _, b := range [][]byte{nil, priv[:len(priv)-1], append(append([]byte(nil), priv...), 0), make([]byte, len(priv)), hpkeFF(len(priv))} {
			_, err := kem.NewPrivateKey(b)
			add(fmt.Sprintf("private %d %s", id, hex.EncodeToString(b)), err)
		}
		enc := pins.get(fmt.Sprintf("enc %d", id), func() []byte {
			e, _ := hpkeMust2(hpke.NewSender(k.PublicKey(), hpke.HKDFSHA256(), hpke.AES128GCM(), nil))
			return e
		}, func(e []byte) bool {
			_, err := hpke.NewRecipient(e, k, hpke.HKDFSHA256(), hpke.AES128GCM(), nil)
			return err == nil
		})
		for _, b := range [][]byte{nil, enc[:len(enc)-1], append(append([]byte(nil), enc...), 0), make([]byte, len(enc)), hpkeFF(len(enc))} {
			_, err := hpke.NewRecipient(b, k, hpke.HKDFSHA256(), hpke.AES128GCM(), nil)
			add(fmt.Sprintf("recipient %d %s", id, hex.EncodeToString(b)), err)
		}
		_, err := hpke.Open(k, hpke.HKDFSHA256(), hpke.AES128GCM(), nil, enc[:len(enc)-1])
		add(fmt.Sprintf("open %d %s", id, hex.EncodeToString(enc[:len(enc)-1])), err)
	}
	for _, l := range []int{-1, 0, 1, 8160, 8161, 12240, 12241, 16320, 16321, 65535, 65536} {
		for _, kdfID := range hpkeKDFs {
			kdf := hpkeMust(hpke.NewKDF(kdfID))
			k := hpkeMust(hpkeMust(hpke.NewKEM(0x0020)).DeriveKeyPair([]byte("export")))
			_, s := hpkeMust2(hpke.NewSender(k.PublicKey(), kdf, hpke.ExportOnly(), nil))
			_, err := s.Export("ctx", l)
			add(fmt.Sprintf("export %d %d", kdfID, l), err)
		}
	}
	{
		k := hpkeMust(hpkeMust(hpke.NewKEM(0x0020)).DeriveKeyPair([]byte("export")))
		_, s := hpkeMust2(hpke.NewSender(k.PublicKey(), hpke.HKDFSHA256(), hpke.ExportOnly(), nil))
		_, err := s.Seal(nil, nil)
		add("exportonly seal", err)
		enc, _ := hpkeMust2(hpke.NewSender(k.PublicKey(), hpke.HKDFSHA256(), hpke.ExportOnly(), nil))
		r := hpkeMust(hpke.NewRecipient(enc, k, hpke.HKDFSHA256(), hpke.ExportOnly(), nil))
		_, err = r.Open(nil, nil)
		add("exportonly open", err)
	}
	// hybrids of keys, and their bytes
	{
		dk768 := hpkeMust(mlkem.NewDecapsulationKey768(make([]byte, 64)))
		dk1024 := hpkeMust(mlkem.NewDecapsulationKey1024(make([]byte, 64)))
		for _, c := range []struct {
			name  string
			curve ecdh.Curve
		}{{"x25519", ecdh.X25519()}, {"p256", ecdh.P256()}, {"p384", ecdh.P384()}, {"p521", ecdh.P521()}} {
			seed := make([]byte, map[string]int{"x25519": 32, "p256": 32, "p384": 48, "p521": 66}[c.name])
			seed[len(seed)-1] = 7
			t := hpkeMust(c.curve.NewPrivateKey(seed))
			k, err := hpke.NewHybridPrivateKey(dk768, t)
			add("hybrid 768 "+c.name, err)
			if err == nil {
				_, err = k.Bytes()
				add("hybrid 768 "+c.name+" bytes", err)
				_, err = hpke.NewHybridPublicKey(dk768.EncapsulationKey(), t.PublicKey())
				add("hybrid public 768 "+c.name, err)
			}
			_, err = hpke.NewHybridPrivateKey(dk1024, t)
			add("hybrid 1024 "+c.name, err)
			_, err = hpke.NewHybridPublicKey(dk1024.EncapsulationKey(), t.PublicKey())
			add("hybrid public 1024 "+c.name, err)
		}
	}
	o.WriteString("]\n")

	o.WriteString(hpkeTests)
	return o.String()
}

func hpkeMust2[A, B any](a A, b B, err error) (A, B) {
	hpkeMust(0, err)
	return a, b
}

func hpkeFlip(b []byte, i int) []byte {
	c := append([]byte(nil), b...)
	c[i] ^= 0x01
	return c
}

func hpkeFF(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = 0xff
	}
	return b
}

const hpkeTests = `
fn hx(s: Str) -> Bytes { Bytes.from_hex(s) ?? fault("hex") }

fn show(bad: List[Str]) -> List[Str] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

/// A Rand that gives fixed bytes (the ephemeral key or message Go's test
/// gives its hooks).
struct Fixed {
  b: Bytes
  at: Mutex[Int]
}

fn fixed(b: Bytes) -> Fixed { Fixed{b: b, at: Mutex[Int].new(0)} }

fn (s: Fixed) int(lo: Int, hi: Int) -> Int uses rand { lo }
fn (s: Fixed) float() -> Float uses rand { 0.5 }
fn (s: Fixed) bool() -> Bool uses rand { false }
fn (s: Fixed) shuffle[T](xs: List[T]) -> List[T] uses rand { xs }
fn (s: Fixed) bytes(n: Int) -> Bytes uses rand {
  let i = s.at.with(fn(var x: Int) -> Int {
    let at = x
    x = x + n
    at
  })
  s.b[i..i + n]
}

fn kem(id: Int) -> hpke.Kem { hpke.new_kem(id) ?? e => fault(e.msg) }
fn kdf(id: Int) -> hpke.Kdf { hpke.new_kdf(id) ?? e => fault(e.msg) }
fn aead(id: Int) -> hpke.Aead { hpke.new_aead(id) ?? e => fault(e.msg) }

/// drawRandomInput of Go's test: a length byte, then that many bytes.
fn draw(var s: crypto.Shake) -> Bytes {
  let n = Int(s.read(1)[0])
  s.read(n)
}

fn check_vector(v: (Int, Int, Int, Bytes, Bytes, Bytes, Bytes, Bytes, Bytes, List[(Bytes, Bytes, Bytes)], List[(Bytes, Int, Bytes)], Str, Str)) -> List[Str] {
  let (kem_id, kdf_id, aead_id, info, rnd, pk_rm, sk_rm, ikm_r, enc_want, encs, exps, acc_enc, acc_exp) = v
  let name = "kem ${kem_id:04x} kdf ${kdf_id:04x} aead ${aead_id:04x}"
  var bad: List[Str] = []
  let k = kem(kem_id)
  let f = kdf(kdf_id)
  let a = aead(aead_id)
  if k.id() != kem_id || f.id() != kdf_id || a.id() != aead_id {
    bad.push("${name}: ids")
  }
  let pk = k.public_key(pk_rm) ?? e => return ["${name}: public key: ${e.msg}"]
  if pk.kem() != k || pk.bytes() != pk_rm {
    bad.push("${name}: public key bytes")
  }
  var s = hpke.new_sender(fixed(rnd), pk, f, a, info) ?? e => return ["${name}: sender: ${e.msg}"]
  if s.enc() != enc_want {
    bad.push("${name}: enc ${s.enc().to_hex()}")
  }
  let sk = k.private_key(sk_rm) ?? e => return ["${name}: private key: ${e.msg}"]
  let skb = sk.bytes() ?? e => return ["${name}: private bytes: ${e.msg}"]
  if skb != sk_rm && k != hpke.Kem.DhkemX25519 {
    bad.push("${name}: private key bytes")
  }
  if k == hpke.Kem.DhkemX25519 {
    let k2 = k.private_key(skb) ?? e => return ["${name}: clamped key: ${e.msg}"]
    if (k2.bytes() ?? _ => b"") != skb || k2.public_key().bytes() != pk_rm {
      bad.push("${name}: X25519 re-serialized key")
    }
  }
  if sk.public_key().bytes() != pk_rm {
    bad.push("${name}: public key of the private key")
  }
  let d = k.derive_key_pair(ikm_r) ?? e => return ["${name}: derive: ${e.msg}"]
  if ((d.bytes() ?? _ => b"") != sk_rm && k != hpke.Kem.DhkemX25519) || d.public_key().bytes() != pk_rm {
    bad.push("${name}: derive_key_pair")
  }
  var r = hpke.new_recipient(s.enc(), sk, f, a, info) ?? e => return ["${name}: recipient: ${e.msg}"]
  if a != hpke.Aead.ExportOnly && acc_enc != "" {
    var source = crypto.shake128()
    var sink = crypto.shake128()
    for _ in 0..1000 {
      let aad = draw(source)
      let pt = draw(source)
      let ct = s.seal(aad, pt) ?? e => return ["${name}: seal: ${e.msg}"]
      sink.write(ct)
      let got = r.open(aad, ct) ?? e => return ["${name}: open: ${e.msg}"]
      if got != pt {
        bad.push("${name}: open of seal")
      }
    }
    if sink.read(16).to_hex() != acc_enc {
      bad.push("${name}: accumulated encryptions")
    }
  } else if a != hpke.Aead.ExportOnly {
    for (aad, pt, ct_want) in encs {
      let ct = s.seal(aad, pt) ?? e => return ["${name}: seal: ${e.msg}"]
      if ct != ct_want {
        bad.push("${name}: ciphertext")
      }
      if (r.open(aad, ct) ?? _ => b"!") != pt {
        bad.push("${name}: open")
      }
    }
  } else {
    if s.seal(b"", b"").is_ok() || r.open(b"", b"").is_ok() {
      bad.push("${name}: export-only sealed or opened")
    }
  }
  if acc_exp != "" {
    var source = crypto.shake128()
    var sink = crypto.shake128()
    for l in 0..1000 {
      let ctx = draw(source)
      let value = s.export(ctx, l) ?? e => return ["${name}: export: ${e.msg}"]
      sink.write(value)
      if (r.export(ctx, l) ?? _ => b"!") != value {
        bad.push("${name}: recipient export")
      }
    }
    if sink.read(16).to_hex() != acc_exp {
      bad.push("${name}: accumulated exports")
    }
  } else {
    for (ctx, l, want) in exps {
      if (s.export(ctx, l) ?? _ => b"!") != want || (r.export(ctx, l) ?? _ => b"!") != want {
        bad.push("${name}: export ${l}")
      }
    }
  }
  bad
}

test "the RFC 9180 and hpke-pq vectors (Go's TestVectors)" {
  var bad: List[Str] = []
  for v in VECTORS {
    for b in check_vector(v) {
      bad.push(b)
    }
  }
  expect(show(bad)) == []
}

test "derive_key_pair is Go's DeriveKeyPair" {
  var bad: List[Str] = []
  for (id, ikm, sk, pk) in DERIVED {
    let k = kem(id).derive_key_pair(ikm) ?? e => fault(e.msg)
    if (k.bytes() ?? _ => b"") != sk || k.public_key().bytes() != pk {
      bad.push("kem ${id:04x} ikm ${ikm.len()}")
    }
  }
  expect(show(bad)) == []
}

test "open of messages Go sealed, changed and cut, is Go's Open" {
  var bad: List[Str] = []
  for (id, ikm, kdf_id, aead_id, info, m, want) in SEALED {
    let k = kem(id).derive_key_pair(ikm) ?? e => fault(e.msg)
    let got = match hpke.open(k, kdf(kdf_id), aead(aead_id), info, m) {
      Ok(p) => p.to_hex()
      Err(e) => "err ${e.msg}"
    }
    if got != want {
      bad.push("kem ${id:04x} kdf ${kdf_id:04x} aead ${aead_id:04x}: ${got}, want ${want}")
    }
  }
  expect(show(bad)) == []
}

fn res[T](r: Result[T, hpke.HpkeErr]) -> Str {
  match r {
    Ok(_) => "ok"
    Err(e) => e.msg
  }
}

fn hybrid_seed(c: crypto.EcdhCurve) -> Bytes {
  let n = match c {
    X25519 => 32
    P256 => 32
    P384 => 48
    P521 => 66
  }
  var b = BytesBuilder.with_capacity(n)
  for _ in 0..n - 1 {
    b.write_byte(0)
  }
  b.write_byte(7)
  b.freeze()
}

fn got_error(what: Str) -> Str {
  let w = what.split(" ")
  let id = Int.parse(w[1]) ?? 0
  let data = if w.len() > 2 { Bytes.from_hex(w[2]) ?? b"" } else { b"" }
  let curves = {"x25519": crypto.EcdhCurve.X25519, "p256": crypto.EcdhCurve.P256, "p384": crypto.EcdhCurve.P384, "p521": crypto.EcdhCurve.P521}
  match w[0] {
    "kem" => res(hpke.new_kem(id))
    "kdf" => res(hpke.new_kdf(id))
    "aead" => res(hpke.new_aead(id))
    "public" => res(kem(id).public_key(data))
    "private" => res(kem(id).private_key(data))
    "recipient" | "open" => {
      var i = 0
      for (j, x) in [0x0010, 0x0011, 0x0012, 0x0020, 0x0041, 0x0042, 0x647a, 0x0050, 0x0051].indexed() {
        if x == id {
          i = j
        }
      }
      let k = kem(id).derive_key_pair("recipient ${i}".bytes()) ?? e => fault(e.msg)
      if w[0] == "open" {
        res(hpke.open(k, hpke.Kdf.HkdfSha256, hpke.Aead.Aes128Gcm, b"", data))
      } else {
        res(hpke.new_recipient(data, k, hpke.Kdf.HkdfSha256, hpke.Aead.Aes128Gcm))
      }
    }
    "export" => {
      let k = hpke.Kem.DhkemX25519.derive_key_pair(b"export") ?? e => fault(e.msg)
      let s = hpke.new_sender(fixed(b"01234567890123456789012345678901"), k.public_key(), kdf(id), hpke.Aead.ExportOnly) ?? e => fault(e.msg)
      res(s.export(b"ctx", Int.parse(w[2]) ?? 0))
    }
    "exportonly" => {
      let k = hpke.Kem.DhkemX25519.derive_key_pair(b"export") ?? e => fault(e.msg)
      var s = hpke.new_sender(fixed(b"01234567890123456789012345678901"), k.public_key(), hpke.Kdf.HkdfSha256, hpke.Aead.ExportOnly) ?? e => fault(e.msg)
      if w[1] == "seal" {
        return res(s.seal(b"", b""))
      }
      var r = hpke.new_recipient(s.enc(), k, hpke.Kdf.HkdfSha256, hpke.Aead.ExportOnly) ?? e => fault(e.msg)
      res(r.open(b"", b""))
    }
    "hybrid" => {
      let c = curves.get(w[2]) ?? fault("curve")
      let p = if w[1] == "768" { crypto.MlkemParams.Mlkem768 } else { crypto.MlkemParams.Mlkem1024 }
      let dk = crypto.mlkem_decapsulation_key(p, hx("00".repeat(64))) ?? e => fault(e.msg)
      let t = crypto.ecdh_private_key(c, hybrid_seed(c)) ?? e => fault(e.msg)
      let k = hpke.hybrid_private_key(dk, t)
      if w.len() > 3 {
        return match k {
          Ok(k) => res(k.bytes())
          Err(e) => e.msg
        }
      }
      res(k)
    }
    _ => fault("unknown case ${what}")
  }
}

test "errors are Go's" {
  var bad: List[Str] = []
  for (what, want) in ERRORS {
    let w = what.split(" ")
    let got = if w[0] == "hybrid" && w[1] == "public" {
      let c = {"x25519": crypto.EcdhCurve.X25519, "p256": crypto.EcdhCurve.P256, "p384": crypto.EcdhCurve.P384, "p521": crypto.EcdhCurve.P521}.get(w[3]) ?? fault("curve")
      let p = if w[2] == "768" { crypto.MlkemParams.Mlkem768 } else { crypto.MlkemParams.Mlkem1024 }
      let dk = crypto.mlkem_decapsulation_key(p, hx("00".repeat(64))) ?? e => fault(e.msg)
      let t = crypto.ecdh_private_key(c, hybrid_seed(c)) ?? e => fault(e.msg)
      res(hpke.hybrid_public_key(dk.encapsulation_key(), t.public_key()))
    } else {
      got_error(what)
    }
    if got != want {
      bad.push("${if what.len() > 80 { what[0..80] } else { what }}: ${got}, want ${want}")
    }
  }
  expect(show(bad)) == []
}

/// Sender.seal and Recipient.open of one suite. Outside the loop of its
/// caller: a match binding of a move only value in a loop is XO0510
/// (~/xo-lead/notes/aws-43/xo0510-loop-match).
fn contexts(r: Fixed, pk: hpke.PublicKey, kk: hpke.PrivateKey, f: hpke.Kdf, a: hpke.Aead, name: Str) -> List[Str] {
  var s = hpke.new_sender(r, pk, f, a, b"info") ?? e => fault(e.msg)
  let ct = s.seal(b"aad", b"plaintext") ?? e => fault(e.msg)
  let ct2 = s.seal(b"aad", b"plaintext") ?? e => fault(e.msg)
  var rc = hpke.new_recipient(s.enc(), kk, f, a, b"info") ?? e => fault(e.msg)
  if ct == ct2 || (rc.open(b"aad", ct) ?? _ => b"!") != b"plaintext" || (rc.open(b"aad", ct2) ?? _ => b"!") != b"plaintext" {
    return ["${name}: Sender.seal and Recipient.open"]
  }
  []
}

test "a round trip on every suite, and the wrong info or a changed byte refused (Go's TestRoundTrip)" {
  var bad: List[Str] = []
  let r = fixed(crypto.sum_shake256(b"round trip", 100000))
  for k in [hpke.Kem.DhkemP256, hpke.Kem.DhkemP384, hpke.Kem.DhkemP521, hpke.Kem.DhkemX25519, hpke.Kem.Mlkem768, hpke.Kem.Mlkem1024, hpke.Kem.Mlkem768P256, hpke.Kem.Mlkem1024P384, hpke.Kem.Mlkem768X25519] {
    let sk = k.generate_key(r)
    let kb = sk.bytes() ?? e => fault(e.msg)
    let kk = k.private_key(kb) ?? e => fault(e.msg)
    if (kk.bytes() ?? _ => b"") != kb {
      bad.push("${k.id():04x}: re-serialized key")
    }
    let pk = k.public_key(sk.public_key().bytes()) ?? e => fault(e.msg)
    if pk.bytes() != sk.public_key().bytes() {
      bad.push("${k.id():04x}: re-serialized public key")
    }
    for f in [hpke.Kdf.HkdfSha256, hpke.Kdf.HkdfSha384, hpke.Kdf.HkdfSha512, hpke.Kdf.Shake128, hpke.Kdf.Shake256] {
      for a in [hpke.Aead.Aes128Gcm, hpke.Aead.Aes256Gcm, hpke.Aead.ChaCha20Poly1305] {
        let name = "${k.id():04x} ${f.id():04x} ${a.id():04x}"
        let c = hpke.seal(r, pk, f, a, b"info", b"plaintext") ?? e => fault(e.msg)
        if (hpke.open(kk, f, a, b"info", c) ?? _ => b"!") != b"plaintext" {
          bad.push("${name}: open")
        }
        if hpke.open(kk, f, a, b"wrong", c).is_ok() {
          bad.push("${name}: opened with the wrong info")
        }
        var cb = BytesBuilder.with_capacity(c.len())
        cb.write(c)
        cb.put_le(c.len() - 1, Int(c[c.len() - 1]) ^ 0xff, 1)
        if hpke.open(kk, f, a, b"info", cb.freeze()).is_ok() {
          bad.push("${name}: opened a changed message")
        }
        let c2 = hpke.seal(r, sk.public_key(), f, a, b"", b"") ?? e => fault(e.msg)
        if (hpke.open(sk, f, a, b"", c2) ?? _ => b"!") != b"" {
          bad.push("${name}: empty message")
        }
        for b in contexts(r, pk, kk, f, a, name) {
          bad.push(b)
        }
      }
    }
  }
  expect(show(bad)) == []
}

test "a failed open keeps the sequence, and messages open only in order" {
  let k = hpke.Kem.DhkemX25519.derive_key_pair(b"order") ?? e => fault(e.msg)
  var s = hpke.new_sender(fixed(b"01234567890123456789012345678901"), k.public_key(), hpke.Kdf.HkdfSha256, hpke.Aead.ChaCha20Poly1305) ?? e => fault(e.msg)
  let a = s.seal(b"", b"one") ?? e => fault(e.msg)
  let b = s.seal(b"", b"two") ?? e => fault(e.msg)
  var r = hpke.new_recipient(s.enc(), k, hpke.Kdf.HkdfSha256, hpke.Aead.ChaCha20Poly1305) ?? e => fault(e.msg)
  expect(res(r.open(b"", b))) == "chacha20poly1305: message authentication failed"
  expect((r.open(b"", a) ?? _ => b"!")) == b"one"
  expect((r.open(b"", b) ?? _ => b"!")) == b"two"
}

test "the identifiers and dhkem" {
  expect(hpke.dhkem(crypto.EcdhCurve.P256)) == hpke.Kem.DhkemP256
  expect(hpke.dhkem(crypto.EcdhCurve.X25519).id()) == 0x0020
  let ek = crypto.mlkem_decapsulation_key(crypto.MlkemParams.Mlkem1024, hx("00".repeat(64))) ?? e => fault(e.msg)
  expect(hpke.mlkem_public_key(ek.encapsulation_key()).kem()) == hpke.Kem.Mlkem1024
  expect(hpke.mlkem_private_key(ek).kem()) == hpke.Kem.Mlkem1024
  let t = crypto.ecdh_private_key(crypto.EcdhCurve.P384, hybrid_seed(crypto.EcdhCurve.P384)) ?? e => fault(e.msg)
  expect(hpke.dhkem_private_key(t).kem()) == hpke.Kem.DhkemP384
  expect(hpke.dhkem_public_key(t.public_key()).kem()) == hpke.Kem.DhkemP384
}
`

// hpkePins keep the outputs Go's HPKE makes from its own randomness (the
// ephemeral keys of NewSender and Seal, which no reader controls: ML-KEM
// draws from Go's DRBG), so a regenerate gives identical bytes
// (stdcorpus -check). Each pinned value is checked by Go again on every
// run (Open gives the plaintext, NewRecipient takes the enc), and one that
// fails, or a new key, is made afresh.
type hpkePins struct {
	path    string
	vals    map[string][]byte
	used    map[string]bool
	changed bool
}

func loadHpkePins() *hpkePins {
	p := &hpkePins{path: filepath.Join("scripts", "stdcorpus", "testdata", "hpke", "pinned.txt"), vals: map[string][]byte{}, used: map[string]bool{}}
	b, _ := os.ReadFile(p.path)
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, " = ")
		if ok {
			p.vals[k] = hpkeHex(v)
		}
	}
	return p
}

func (p *hpkePins) get(key string, make func() []byte, ok func([]byte) bool) []byte {
	p.used[key] = true
	if v, have := p.vals[key]; have && ok(v) {
		return v
	}
	v := make()
	if !ok(v) {
		panic("hpke: Go refuses its own output for " + key)
	}
	p.vals[key] = v
	p.changed = true
	return v
}

func (p *hpkePins) save() {
	if len(p.used) != len(p.vals) {
		p.changed = true
	}
	if !p.changed || checkOnly {
		return
	}
	keys := make([]string, 0, len(p.used))
	for k := range p.used {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Go's HPKE outputs made from its own randomness, pinned by scripts/stdcorpus/hpke.go (each checked by Go on every run)\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s = %x\n", k, p.vals[k])
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(p.path, []byte(b.String()), 0o644); err != nil {
		panic(err)
	}
}
