// A differential of the two native AEAD kernels, AES-GCM
// (std/native/llrt/aesgcm.xo) and ChaCha20-Poly1305
// (std/native/llrt/chacha.xo), against Go (decision 0202's B31: std/rt's
// internal_crypto.go is Go's reference for the native TLS primitives).
//
// The golden it generates, testdata/golden/aead_diff, is a self-checking
// program: the case table carries Go's expected sealed text as a
// constant, so the program compares on any machine a binary can run on
// (linux/amd64 and windows in Docker and CI included) with no Go
// toolchain at run time. This test is what keeps the table Go's: it
// rebuilds the program text from the oracle and fails when the committed
// main.xo differs, and -update-aead rewrites it.
//
// The oracle differs by area and the program says so. AES-GCM is
// crypto/cipher's GCM with crypto/aes, Go's own. ChaCha20-Poly1305 is
// std/rt's Go twin, because Go's standard library has no
// ChaCha20-Poly1305; to keep the twin honest every AES case also seals
// through it and requires crypto/cipher's answer, so a drift between the
// two Go implementations fails here rather than being recorded.
//
// The cases are every plaintext length from 0 to 4096 that the kernels
// switch paths on: the 16 byte block, aesgcm.xo's 64 byte chunk, the 128
// and 256 byte widths the AES-NI, PCLMUL, SSSE3, AVX2 and NEON paths work
// at once, and each of those plus or minus one. Each case checks seal,
// open and the opens that must fail on both sides: a bit flipped at the
// first, middle and last sealed byte, a tag cut by a byte, a changed AAD
// and a changed nonce. Go's answers to those are asserted at generation
// time (generateAeadCases panics when Go opens any of them), the program
// asserts the native kernels' at run time.
package llvm

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/std/rt"
)

// updateAead rewrites the aead_diff golden's main.xo from the Go oracle.
var updateAead = flag.Bool("update-aead", false, "rewrite testdata/golden/aead_diff/main.xo from the Go oracle")

// aeadDiffDir is the golden this test generates.
var aeadDiffDir = filepath.Join("testdata", "golden", "aead_diff")

// aeadAlg is one AEAD the cases cover: the crypto_op area, the key size in
// bits (the case name's) and the key size in bytes.
type aeadAlg struct {
	area string
	bits int
	key  int
}

var aeadAlgs = []aeadAlg{
	{"aes", 128, 16},
	{"aes", 192, 24},
	{"aes", 256, 32},
	{"chacha", 256, 32},
}

// aeadLengths are the plaintext lengths every algorithm runs at.
var aeadLengths = []int{
	0, 1, 2, 15, 16, 17, 31, 32, 33, 63, 64, 65, 95, 96, 97,
	127, 128, 129, 191, 192, 193, 255, 256, 257, 383, 384, 385,
	511, 512, 513, 767, 768, 769, 1023, 1024, 1025,
	1535, 1536, 1537, 2047, 2048, 2049, 3071, 3072, 3073, 4095, 4096,
}

// aeadAADLengths are the additional data lengths the cases spread over.
var aeadAADLengths = []int{0, 1, 15, 16, 17, 20, 32, 63, 64, 65, 128, 129, 255}

// aeadCase is one program line: Go's sealed text for these inputs.
type aeadCase struct {
	name, key, nonce, aad, pt, sealed string
}

// aeadSeal is Go's sealed text: ciphertext then tag.
func aeadSeal(alg aeadAlg, key, nonce, pt, aad []byte) []byte {
	if alg.area == "aes" {
		blk, err := aes.NewCipher(key)
		if err != nil {
			panic(err)
		}
		g, err := cipher.NewGCM(blk)
		if err != nil {
			panic(err)
		}
		return g.Seal(nil, nonce, pt, aad)
	}
	return []byte(rt.Internal_crypto_op(nil, "chacha.seal", rt.Bytes(key), rt.Bytes(nonce), rt.Bytes(pt), rt.Bytes(aad)))
}

// aeadOpen is Go's open: the plaintext and true, or nil and false.
func aeadOpen(alg aeadAlg, key, nonce, sealed, aad []byte) ([]byte, bool) {
	if alg.area == "aes" {
		blk, err := aes.NewCipher(key)
		if err != nil {
			panic(err)
		}
		g, err := cipher.NewGCM(blk)
		if err != nil {
			panic(err)
		}
		pt, err := g.Open(nil, nonce, sealed, aad)
		return pt, err == nil
	}
	r := rt.Internal_crypto_op(nil, "chacha.open", rt.Bytes(key), rt.Bytes(nonce), rt.Bytes(sealed), rt.Bytes(aad))
	if len(r) == 0 || r[0] != 1 {
		return nil, false
	}
	return []byte(r[1:]), true
}

// aeadFlip is sealed with one bit of byte i flipped.
func aeadFlip(sealed []byte, i int) []byte {
	out := append([]byte(nil), sealed...)
	out[i] ^= 1
	return out
}

// aeadRFCVectors pins std/rt's ChaCha20-Poly1305, the corpus's oracle for
// the area, to RFC 8439's published vectors: section 2.8.2's seal (the
// "Ladies and Gentlemen" plaintext, key 80..9f, nonce 070000004041424344454647,
// AAD 50515253c0c1c2c3c4c5c6c7) and appendix A.5's open. Without this the
// table would be Go code agreeing with Go code; with it the Go twin is
// anchored to an outside source, the way the AES cases anchor it to
// crypto/cipher. A mismatch panics: generation fails instead of recording
// an oracle nothing outside the repo ever confirmed.
func aeadRFCVectors() {
	unhex := func(s string) []byte {
		b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
		if err != nil {
			panic(fmt.Sprintf("aead rfc vectors: %v", err))
		}
		return b
	}
	key := unhex("808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9f")
	nonce := unhex("070000004041424344454647")
	aad := unhex("50515253c0c1c2c3c4c5c6c7")
	pt := []byte("Ladies and Gentlemen of the class of '99: If I could offer you only one tip for the future, sunscreen would be it.")
	// RFC 8439 2.8.2
	want := unhex("d31a8d34648e60db7b86afbc53ef7ec2a4aded51296e08fea9e2b5a736ee62d6" +
		"3dbea45e8ca9671282fafb69da92728b1a71de0a9e060b2905d6a5b67ecd3b36" +
		"92ddbd7f2d778b8c9803aee328091b58fab324e4fad675945585808b4831d7bc" +
		"3ff4def08e4b7a9de576d26586cec64b6116" +
		"1ae10b594f09e26a7e902ecbd0600691")
	alg := aeadAlg{"chacha", 256, 32}
	if got := aeadSeal(alg, key, nonce, pt, aad); !bytes.Equal(got, want) {
		panic("aead rfc vectors: std/rt's ChaCha20-Poly1305 does not give RFC 8439 section 2.8.2")
	}
	// RFC 8439 A.5
	key5 := unhex("1c9240a5eb55d38af333888604f6b5f0473917c1402b80099dca5cbc207075c0")
	nonce5 := unhex("000000000102030405060708")
	aad5 := unhex("f33388860000000000004e91")
	sealed5 := unhex("64a0861575861af460f062c79be643bd5e805cfd345cf389f108670ac76c8cb2" +
		"4c6cfc18755d43eea09ee94e382d26b0bdb7b73c321b0100d4f03b7f355894cf" +
		"332f830e710b97ce98c8a84abd0b948114ad176e008d33bd60f982b1ff37c855" +
		"9797a06ef4f0ef61c186324e2b3506383606907b6a7c02b0f9f6157b53c867e4" +
		"b9166c767b804d46a59b5216cde7a4e99040c5a40433225ee282a1b0a06c523e" +
		"af4534d7f83fa1155b0047718cbc546a0d072b04b3564eea1b422273f548271a" +
		"0bb2316053fa76991955ebd63159434ecebb4e466dae5a1073a6727627097a10" +
		"49e617d91d361094fa68f0ff77987130305beaba2eda04df997b714d6c6f2c29" +
		"a6ad5cb4022b02709b" +
		"eead9d67890cbb22392336fea1851f38")
	plain5 := unhex("496e7465726e65742d4472616674732061726520647261667420646f63756d65" +
		"6e74732076616c696420666f722061206d6178696d756d206f6620736978206d" +
		"6f6e74687320616e64206d617920626520757064617465642c207265706c6163" +
		"65642c206f72206f62736f6c65746564206279206f7468657220646f63756d65" +
		"6e747320617420616e792074696d652e20497420697320696e617070726f7072" +
		"6961746520746f2075736520496e7465726e65742d4472616674732061732072" +
		"65666572656e6365206d6174657269616c206f7220746f206369746520746865" +
		"6d206f74686572207468616e206173202fe2809c776f726b20696e2070726f67" +
		"726573732e2fe2809d")
	if got, ok := aeadOpen(alg, key5, nonce5, sealed5, aad5); !ok || !bytes.Equal(got, plain5) {
		panic("aead rfc vectors: std/rt's ChaCha20-Poly1305 does not open RFC 8439 appendix A.5")
	}
}

// aeadCases builds the table and asserts Go's side of every check: it
// opens its own seal, and rejects each changed bit, the cut tag, the
// changed AAD and the changed nonce. A panic here fails generation
// instead of recording an oracle the two sides never had.
func aeadCases() []aeadCase {
	aeadRFCVectors()
	r := rand.New(rand.NewSource(89))
	var cases []aeadCase
	randBytes := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(r.Intn(256))
		}
		return b
	}
	hx := func(b []byte) string { return hex.EncodeToString(b) }
	for _, alg := range aeadAlgs {
		name := fmt.Sprintf("%s%d", alg.area, alg.bits)
		for _, size := range aeadLengths {
			key := randBytes(alg.key)
			nonce := randBytes(12)
			aad := randBytes(aeadAADLengths[r.Intn(len(aeadAADLengths))])
			pt := randBytes(size)
			sealed := aeadSeal(alg, key, nonce, pt, aad)
			if got, ok := aeadOpen(alg, key, nonce, sealed, aad); !ok || !bytes.Equal(got, pt) {
				panic(fmt.Sprintf("%s %d bytes: Go does not open its own seal", name, size))
			}
			for _, i := range []int{0, len(sealed) / 2, len(sealed) - 1} {
				if _, ok := aeadOpen(alg, key, nonce, aeadFlip(sealed, i), aad); ok {
					panic(fmt.Sprintf("%s %d bytes: Go opens a seal with byte %d flipped", name, size, i))
				}
			}
			if _, ok := aeadOpen(alg, key, nonce, sealed[:len(sealed)-1], aad); ok {
				panic(fmt.Sprintf("%s %d bytes: Go opens a seal with the tag cut", name, size))
			}
			if _, ok := aeadOpen(alg, key, nonce, sealed, append(append([]byte(nil), aad...), 0)); ok {
				panic(fmt.Sprintf("%s %d bytes: Go opens a seal with a changed AAD", name, size))
			}
			wrong := append([]byte(nil), nonce...)
			wrong[11] ^= 1
			if _, ok := aeadOpen(alg, key, wrong, sealed, aad); ok {
				panic(fmt.Sprintf("%s %d bytes: Go opens a seal with a changed nonce", name, size))
			}
			if alg.area == "aes" {
				// std/rt's Go twin must agree with crypto/cipher, so the two
				// Go implementations cannot drift apart behind the corpus
				twin := rt.Internal_crypto_op(nil, "aes.seal", rt.Bytes(key), rt.Bytes(nonce), rt.Bytes(pt), rt.Bytes(aad))
				if !bytes.Equal([]byte(twin), sealed) {
					panic(fmt.Sprintf("%s %d bytes: std/rt's Go AES-GCM and crypto/cipher disagree", name, size))
				}
			}
			cases = append(cases, aeadCase{name, hx(key), hx(nonce), hx(aad), hx(pt), hx(sealed)})
		}
	}
	return cases
}

// aeadProgram is the golden's main.xo: the table, then the run.
func aeadProgram() string {
	cases := aeadCases()
	var b strings.Builder
	b.WriteString(`// The two native AEAD kernels against Go: AES-128/192/256-GCM
// (std/native/llrt/aesgcm.xo) and ChaCha20-Poly1305
// (std/native/llrt/chacha.xo), through internal.crypto_op, at every
// plaintext length from 0 to 4096 the block, chunk, 128 and 256 byte
// paths switch on. Each case seals, opens, and requires the opens Go
// rejects (a bit flipped at the first, middle and last sealed byte, a tag
// cut by a byte, a changed AAD, a changed nonce) to fail here too.
//
// Generated by internal/llvm TestAeadDiffTable (-update-aead); the table
// is Go's, so do not edit it by hand. AES-GCM's oracle is crypto/cipher's
// GCM; ChaCha20-Poly1305's is std/rt's Go twin, Go's standard library
// having none, which the generator requires to open and seal RFC 8439
// section 2.8.2 and appendix A.5's published vectors, and requires to
// agree with crypto/cipher on every AES case.
use std/internal

/// (algorithm, key hex, nonce hex, AAD hex, plaintext hex, Go's sealed hex)
const CASES: List[(Str, Str, Str, Str, Str, Str)] = [
`)
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%s, %s, %s, %s, %s, %s),\n", quoteXo(c.name), quoteXo(c.key), quoteXo(c.nonce), quoteXo(c.aad), quoteXo(c.pt), quoteXo(c.sealed))
	}
	b.WriteString(`]

fn hx(s: Str) -> Bytes { Bytes.from_hex(s) ?? b"" }

/// sealed hex with one bit of the byte at hex offset i flipped (a low
/// nibble flip), so a changed bit reaches the kernel as changed bytes.
fn fl(h: Str, i: Int) -> Str {
  let c = h[i..(i + 1)]
  let f = if c == "0" { "1" } else { "0" }
  "${h[0..i]}${f}${h[(i + 1)..h.len()]}"
}

/// internal.crypto_op's open answer as text: the plaintext in hex, or
/// "fail" for the 0 byte a tag that does not match gives.
fn opened(r: Bytes) -> Str {
  let h = r.to_hex()
  if h == "00" { "fail" } else if h.starts_with("01") { h[2..h.len()] } else { "bad ${h}" }
}

fn main(os: Os) {
  var bad = 0
  for (alg, key, nonce, aad, pt, want) in CASES {
    let k = hx(key)
    let n = hx(nonce)
    let a = hx(aad)
    let p = hx(pt)
    let area = alg[0..(if alg.starts_with("aes") { 3 } else { 6 })]
    let what = "${alg} ${pt.len() / 2}"
    let before = bad
    let got = internal.crypto_op("${area}.seal", k, n, p, a).to_hex()
    if got != want {
      bad += 1
      os.stdio.println("${what} seal: got ${got} want ${want}")
    }
    let back = opened(internal.crypto_op("${area}.open", k, n, hx(want), a))
    if back != pt {
      bad += 1
      os.stdio.println("${what} open: got ${back}")
    }
    for i in [0, want.len() / 2, want.len() - 1] {
      let f = opened(internal.crypto_op("${area}.open", k, n, hx(fl(want, i)), a))
      if f != "fail" {
        bad += 1
        os.stdio.println("${what} byte ${i} flipped opened as ${f}")
      }
    }
    let cut = opened(internal.crypto_op("${area}.open", k, n, hx(want[0..(want.len() - 2)]), a))
    if cut != "fail" {
      bad += 1
      os.stdio.println("${what} a tag cut by a byte opened as ${cut}")
    }
    let other_aad = if aad.len() > 0 { fl(aad, 0) } else { "00" }
    let with_aad = opened(internal.crypto_op("${area}.open", k, n, hx(want), hx(other_aad)))
    if with_aad != "fail" {
      bad += 1
      os.stdio.println("${what} a changed AAD opened as ${with_aad}")
    }
    let with_nonce = opened(internal.crypto_op("${area}.open", k, hx(fl(nonce, 0)), hx(want), a))
    if with_nonce != "fail" {
      bad += 1
      os.stdio.println("${what} a changed nonce opened as ${with_nonce}")
    }
    if bad == before {
      os.stdio.println("${what} ok")
    }
  }
  os.stdio.println("aead ${CASES.len()} cases, ${bad} failed")
}
`)
	return b.String()
}

// quoteXo is s as an Xo string literal (the case table holds hex only, so
// this is the plain case).
func quoteXo(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '$':
			b.WriteString(`\$`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// TestAeadDiffTable checks that the committed golden program is the one
// the Go oracle produces now: a changed oracle, a changed length list or a
// hand edit fails here, and -update-aead rewrites it. It builds no
// program, so it runs under -short too.
func TestAeadDiffTable(t *testing.T) {
	path := filepath.Join(aeadDiffDir, "main.xo")
	want := aeadProgram()
	if *updateAead {
		if err := os.MkdirAll(aeadDiffDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update-aead)", err)
	}
	if string(got) != want {
		t.Fatalf("%s is not what the Go oracle generates now (run with -update-aead, then check the golden): it holds %d bytes, the oracle %d", path, len(got), len(want))
	}
}
