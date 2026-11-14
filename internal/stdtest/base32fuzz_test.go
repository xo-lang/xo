// A fuzz harness for std/base32 against Go's encoding/base32 (decision
// 0616's fuzz kind). Each case is one of the four encodings (Std, Hex,
// RawStd, RawHex as Go's StdEncoding, HexEncoding and both
// WithPadding(NoPadding)), random bytes (bxData of base64fuzz_test.go),
// Go's EncodeToString of them, and a text to decode near that encoding
// (baseText: \r and \n put in, cut, a byte replaced by a lower case
// letter, a digit outside the alphabet, `=`, a space or multi-byte text,
// padding dropped or added, two encodings joined, a random string over the
// alphabet):
//
//   - encode gives what EncodeToString gives, and decode of it the bytes;
//   - decode of the text gives DecodeString's bytes, or its
//     CorruptInputError text (DecodeErr.display, the offset in the text
//     without \r and \n, as Go's).
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/base32fuzz/base32fuzz_test.xo, which TestStdCorpora runs
// on every corpus backend; TestBase32FuzzCorpusCurrent checks it is what
// this generator writes (XO_BASE32FUZZ_WRITE=1 rewrites it). TestBase32Fuzz
// runs a fresh seed (XO_BASE32FUZZ_SEED, default 2; XO_BASE32FUZZ_N cases,
// default 2000) on XO_BASE32FUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"encoding/base32"
	"math/rand"
	"testing"
)

// b32Encodings are std/base32's VARIANTS in order.
var b32Encodings = []*base32.Encoding{base32.StdEncoding, base32.HexEncoding, base32.StdEncoding.WithPadding(base32.NoPadding), base32.HexEncoding.WithPadding(base32.NoPadding)}

func b32Cases(r *rand.Rand, n int) []bxCase {
	const std = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	const hexa = "0123456789ABCDEFGHIJKLMNOPQRSTUV"
	cs := make([]bxCase, 0, n)
	for len(cs) < n {
		v := r.Intn(4)
		e := b32Encodings[v]
		d := bxData(r)
		c := bxCase{variant: v, data: d, enc: e.EncodeToString(d)}
		alphabet, other := std, "abcz0189+/-_"
		if v == 1 || v == 3 {
			alphabet, other = hexa, "abvwWXYZ+/-_"
		}
		c.text = baseText(r, c.enc, alphabet, other, e.EncodeToString(bxShort(r)), v == 0 || v == 1)
		c.want = bxAnswer(e.DecodeString(c.text))
		cs = append(cs, c)
	}
	return cs
}

const b32CheckXo = `use std/base32

const VARIANTS: List[base32.Base32] = [base32.Base32.Std, base32.Base32.Hex, base32.Base32.RawStd, base32.Base32.RawHex]

fn answer(r: Result[Bytes, base32.DecodeErr]) -> Str {
  match r {
    Ok(b) => "ok ${b.to_hex()}"
    Err(e) => "err ${e.display()}"
  }
}

test "std/base32 encodes as Go's EncodeToString" {
  var bad: List[Str] = []
  for (v, h, enc, _, _) in CASES {
    let d = Bytes.from_hex(h).or_fault("hex")
    if base32.encode(d, VARIANTS[v]) != enc || answer(base32.decode(enc, VARIANTS[v])) != "ok ${h}" {
      bad.push("${v} ${h}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${CASES.len()} inputs encode differently, first (variant, bytes): ${bad[0]}")
  }
}

test "std/base32 decodes as Go's DecodeString" {
  var bad: List[Str] = []
  for (v, _, _, t, want) in CASES {
    let s = Bytes.from_hex(t).or_fault("hex").to_str() ?? ""
    let got = answer(base32.decode(s, VARIANTS[v]))
    if got != want {
      bad.push("${v} ${t}: ${got}, Go ${want}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${CASES.len()} texts decode differently, first (variant, text hex): ${bad[0]}")
  }
}
`

const b32Corpus = "testdata/corpus/base32fuzz/base32fuzz_test.xo"

// TestBase32FuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestBase32FuzzCorpusCurrent(t *testing.T) {
	cases := b32Cases(rand.New(rand.NewSource(1)), 500)
	bxCoverage(t, cases)
	bxCorpusCurrent(t, b32Corpus, "XO_BASE32FUZZ_WRITE", bxXoFile("base32", "encoding/base32", b32CheckXo, 1, cases))
}

// TestBase32Fuzz runs a fresh seed's cases.
func TestBase32Fuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the base32 fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_BASE32FUZZ_SEED", 2))
	n := envInt(t, "XO_BASE32FUZZ_N", 2000)
	cases := b32Cases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "base32fuzz_test.xo", bxXoFile("base32", "encoding/base32", b32CheckXo, seed, cases), envBackends("XO_BASE32FUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
