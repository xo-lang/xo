package main

import (
	"crypto/mlkem"
	"crypto/mlkem/mlkemtest"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
)

// ML-KEM (decision 0604): crypto/mlkem on seeded seeds and messages for
// both parameter sets; encapsulation keys with bytes set or cut (an
// unreduced coefficient is Go's `invalid polynomial encoding`);
// ciphertexts tampered (the implicit rejection key) or of the wrong
// length. Long values are compared by SHA-256.

type mlkemSet struct {
	ek   func(seed []byte) []byte
	enc  func(ek []byte, m []byte) ([]byte, []byte, error)
	dec  func(seed, c []byte) ([]byte, error)
	newE func(ek []byte) error
	ekN  int
	ctN  int
}

var mlkemSets = []mlkemSet{
	{
		ek: func(seed []byte) []byte {
			k, err := mlkem.NewDecapsulationKey768(seed)
			if err != nil {
				panic(err)
			}
			return k.EncapsulationKey().Bytes()
		},
		enc: func(ek, m []byte) ([]byte, []byte, error) {
			e, err := mlkem.NewEncapsulationKey768(ek)
			if err != nil {
				return nil, nil, err
			}
			return mlkemtest.Encapsulate768(e, m)
		},
		dec: func(seed, c []byte) ([]byte, error) {
			k, _ := mlkem.NewDecapsulationKey768(seed)
			return k.Decapsulate(c)
		},
		newE: func(ek []byte) error { _, err := mlkem.NewEncapsulationKey768(ek); return err },
		ekN:  mlkem.EncapsulationKeySize768, ctN: mlkem.CiphertextSize768,
	},
	{
		ek: func(seed []byte) []byte {
			k, err := mlkem.NewDecapsulationKey1024(seed)
			if err != nil {
				panic(err)
			}
			return k.EncapsulationKey().Bytes()
		},
		enc: func(ek, m []byte) ([]byte, []byte, error) {
			e, err := mlkem.NewEncapsulationKey1024(ek)
			if err != nil {
				return nil, nil, err
			}
			return mlkemtest.Encapsulate1024(e, m)
		},
		dec: func(seed, c []byte) ([]byte, error) {
			k, _ := mlkem.NewDecapsulationKey1024(seed)
			return k.Decapsulate(c)
		},
		newE: func(ek []byte) error { _, err := mlkem.NewEncapsulationKey1024(ek); return err },
		ekN:  mlkem.EncapsulationKeySize1024, ctN: mlkem.CiphertextSize1024,
	},
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// mlkemEdit is an edit as the Xo test applies it: "" (none), "len N"
// (cut, or padded with zeros), or "set I X" (byte I becomes X).
func mlkemEdit(r *rand.Rand, n int, tPart int, mostlyUnreduced bool) string {
	switch k := r.Intn(10); {
	case k == 0:
		return ""
	case k <= 2:
		ls := []int{0, 1, n - 1, n + 1, n - 32, n + 32, r.Intn(n)}
		return fmt.Sprintf("len %d", ls[r.Intn(len(ls))])
	case k <= 6 && mostlyUnreduced:
		// a byte of t set high: the coefficient is often 3329 or above
		return fmt.Sprintf("set %d %d", r.Intn(tPart), 0xd0+r.Intn(0x30))
	}
	return fmt.Sprintf("set %d %d", r.Intn(n), r.Intn(256))
}

func mlkemApply(b []byte, e string) []byte {
	out := append([]byte(nil), b...)
	var i, x int
	if _, err := fmt.Sscanf(e, "len %d", &i); err == nil {
		if i <= len(out) {
			return out[:i]
		}
		return append(out, make([]byte, i-len(out))...)
	}
	if _, err := fmt.Sscanf(e, "set %d %d", &i, &x); err == nil {
		out[i] = byte(x)
	}
	return out
}

func mlkemCorpus() string {
	r := rand.New(rand.NewSource(604))
	rb := func(n int) []byte { b := make([]byte, n); r.Read(b); return b }
	var o strings.Builder
	o.WriteString(header("crypto/mlkem (ML-KEM-768 and ML-KEM-1024) on seeded seeds and messages, edited encapsulation keys, and tampered or cut ciphertexts"))
	o.WriteString("use std/crypto\n\nconst PARAMS: List[crypto.MlkemParams] = [crypto.MlkemParams.Mlkem768, crypto.MlkemParams.Mlkem1024]\n\n/// (params, seed hex, Go's NewDecapsulationKey: ok SHA256(encapsulation key) or err TEXT)\nconst KEYS: List[(Int, Str, Str)] = [\n")
	for k := 0; k < 80; k++ {
		p := r.Intn(2)
		n := 64
		if r.Intn(8) == 0 {
			n = []int{0, 32, 63, 65, 96}[r.Intn(5)]
		}
		seed := rb(n)
		got := ""
		if n != 64 {
			_, err := mlkem.NewDecapsulationKey768(seed)
			got = "err " + err.Error()
		} else {
			got = "ok " + sha(mlkemSets[p].ek(seed))
		}
		fmt.Fprintf(&o, "  (%d, %s, %s),\n", p, xq(hex.EncodeToString(seed)), xq(got))
	}
	o.WriteString("]\n\n/// (params, seed hex, message hex, Go's mlkemtest.Encapsulate: shared key hex, SHA256(ciphertext))\nconst ENC: List[(Int, Str, Str, Str, Str)] = [\n")
	for k := 0; k < 80; k++ {
		p := r.Intn(2)
		seed, m := rb(64), rb(32)
		s, c, err := mlkemSets[p].enc(mlkemSets[p].ek(seed), m)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(&o, "  (%d, %s, %s, %s, %s),\n", p, xq(hex.EncodeToString(seed)), xq(hex.EncodeToString(m)), xq(hex.EncodeToString(s)), xq(sha(c)))
	}
	o.WriteString("]\n\n/// (params, seed hex, edit of its encapsulation key, Go's NewEncapsulationKey: ok, or err TEXT)\nconst EKS: List[(Int, Str, Str, Str)] = [\n")
	for k := 0; k < 160; k++ {
		p := r.Intn(2)
		seed := rb(64)
		ek := mlkemSets[p].ek(seed)
		e := mlkemEdit(r, len(ek), len(ek)-32, true)
		got := "ok"
		if err := mlkemSets[p].newE(mlkemApply(ek, e)); err != nil {
			got = "err " + err.Error()
		}
		fmt.Fprintf(&o, "  (%d, %s, %s, %s),\n", p, xq(hex.EncodeToString(seed)), xq(e), xq(got))
	}
	o.WriteString("]\n\n/// (params, seed hex, message hex, edit of the ciphertext, Go's Decapsulate: ok SHARED_HEX or err TEXT)\nconst DEC: List[(Int, Str, Str, Str, Str)] = [\n")
	for k := 0; k < 120; k++ {
		p := r.Intn(2)
		seed, m := rb(64), rb(32)
		_, c, _ := mlkemSets[p].enc(mlkemSets[p].ek(seed), m)
		e := mlkemEdit(r, len(c), len(c), false)
		got := ""
		if s, err := mlkemSets[p].dec(seed, mlkemApply(c, e)); err != nil {
			got = "err " + err.Error()
		} else {
			got = "ok " + hex.EncodeToString(s)
		}
		fmt.Fprintf(&o, "  (%d, %s, %s, %s, %s),\n", p, xq(hex.EncodeToString(seed)), xq(hex.EncodeToString(m)), xq(e), xq(got))
	}
	o.WriteString("]\n")
	o.WriteString(mlkemTests)
	return o.String()
}

const mlkemTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

fn hx(s: Str) -> Bytes { Bytes.from_hex(s) ?? fault("hex") }

fn sha(b: Bytes) -> Str { crypto.digest(crypto.Hash.Sha256, b).to_hex() }

/// A Rand that gives fixed bytes (the seed or message Go was given).
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

/// An edit as the generator's mlkemApply makes it.
fn apply(b: Bytes, e: Str) -> Bytes {
  let w = e.split(" ")
  if w.len() < 2 {
    return b
  }
  var out = BytesBuilder.new()
  if w[0] == "len" {
    let n = Int.parse(w[1]) ?? fault("len")
    if n <= b.len() {
      return b[0..n]
    }
    out.write(b)
    for _ in 0..n - b.len() {
      out.write_byte(UInt8(0))
    }
    return out.freeze()
  }
  let i = Int.parse(w[1]) ?? fault("set")
  let x = Int.parse(w[2]) ?? fault("set")
  out.write(b[0..i])
  out.write_byte(UInt8(x))
  out.write(b[i + 1..b.len()])
  out.freeze()
}

fn dk(p: Int, seed: Str) -> crypto.MlkemDecapsulationKey { crypto.mlkem_decapsulation_key(PARAMS[p], hx(seed)).ok() ?? fault("seed") }

test "decapsulation keys are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (p, seed, want) in KEYS {
    let got = match crypto.mlkem_decapsulation_key(PARAMS[p], hx(seed)) {
      Ok(k) => {
        let e = k.encapsulation_key()
        if k.bytes() != hx(seed) || k.params() != PARAMS[p] || e.params() != PARAMS[p] || e.bytes().len() != PARAMS[p].encapsulation_key_size() {
          "round trip"
        } else {
          "ok ${sha(e.bytes())}"
        }
      }
      Err(e) => "err ${e.msg}"
    }
    if got != want {
      bad.push((seed, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "encapsulation is Go's and decapsulates" {
  var bad: List[(Str, Str, Str)] = []
  for (p, seed, m, shared, ct) in ENC {
    let k = dk(p, seed)
    let (s, c) = k.encapsulation_key().encapsulate(fixed(hx(m)))
    let back = k.decapsulate(c).ok() ?? b""
    let got = "${s.to_hex()} ${sha(c)} ${c.len() == PARAMS[p].ciphertext_size()} ${back == s}"
    let want = "${shared} ${ct} true true"
    if got != want {
      bad.push((m, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "encapsulation keys are checked as Go checks them" {
  var bad: List[(Str, Str, Str)] = []
  for (p, seed, edit, want) in EKS {
    let ek = apply(dk(p, seed).encapsulation_key().bytes(), edit)
    let got = match crypto.mlkem_encapsulation_key(PARAMS[p], ek) {
      Ok(e) => if e.bytes() == ek { "ok" } else { "round trip" }
      Err(e) => "err ${e.msg}"
    }
    if got != want {
      bad.push((edit, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "decapsulation of tampered and cut ciphertexts is Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (p, seed, m, edit, want) in DEC {
    let k = dk(p, seed)
    let (_s, c) = k.encapsulation_key().encapsulate(fixed(hx(m)))
    let got = match k.decapsulate(apply(c, edit)) {
      Ok(s) => "ok ${s.to_hex()}"
      Err(e) => "err ${e.msg}"
    }
    if got != want {
      bad.push((edit, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "keys from one seed are equal, and a decapsulation key's Debug shows no key" {
  let (p, seed, _m, _s, _c) = ENC[0]
  expect(dk(p, seed).encapsulation_key() == dk(p, seed).encapsulation_key()) == true
  expect(dk(p, seed).debug()) == "<mlkemdecapsulationkey>"
  expect(crypto.mlkem_generate_key(fixed(hx(seed)), PARAMS[p]).bytes()) == hx(seed)
  expect((crypto.MLKEM_SHARED_KEY_SIZE, crypto.MLKEM_SEED_SIZE)) == (32, 64)
  expect(PARAMS.map(fn(q) { (q.encapsulation_key_size(), q.ciphertext_size()) })) == [(1184, 1088), (1568, 1568)]
}
`
