// A fuzz harness for std/base64 against Go's encoding/base64 (decision
// 0616's fuzz kind). Each case is one of the four encodings (Std, Url,
// RawStd, RawUrl as Go's StdEncoding, URLEncoding, RawStdEncoding and
// RawURLEncoding), random bytes (0 to 300 of them, from runs of one byte,
// all 256 values, text and 0x00 0xff pairs), Go's EncodeToString of them,
// and a text to decode near that encoding (baseText: the encoding itself,
// with \r and \n put in, cut, a byte replaced by one of another alphabet,
// by `=`, by a space or by multi-byte text, padding dropped or added, two
// encodings joined, or a random string over the alphabet):
//
//   - encode gives what EncodeToString gives, and decode of it the bytes;
//     for Std, Bytes.to_base64 gives the same text;
//   - decode of the text gives DecodeString's bytes, or its
//     CorruptInputError text (DecodeErr.display); for Std, a Some from
//     Bytes.from_base64 holds DecodeString's bytes.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/base64fuzz/base64fuzz_test.xo, which TestStdCorpora runs
// on every corpus backend; TestBase64FuzzCorpusCurrent checks it is what
// this generator writes (XO_BASE64FUZZ_WRITE=1 rewrites it). TestBase64Fuzz
// runs a fresh seed (XO_BASE64FUZZ_SEED, default 2; XO_BASE64FUZZ_N cases,
// default 2000) on XO_BASE64FUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bxCase is one case of the base32 and base64 harnesses: the encoding
// (an index into the module's VARIANTS), the bytes, Go's encoding of
// them, a text to decode and Go's answer for it ("ok HEX" or "err TEXT").
type bxCase struct {
	variant int
	data    []byte
	enc     string
	text    string
	want    string
}

// bxData is 0 to 300 bytes, a quarter of them 0 to 8.
func bxData(r *rand.Rand) []byte {
	n := r.Intn(301)
	if r.Intn(4) == 0 {
		n = r.Intn(9)
	} else if r.Intn(2) == 0 {
		n = r.Intn(64)
	}
	b := make([]byte, 0, n)
	for len(b) < n {
		switch r.Intn(5) {
		case 0:
			c := byte(r.Intn(256))
			for k := r.Intn(20); k > 0 && len(b) < n; k-- {
				b = append(b, c)
			}
		case 1:
			b = append(b, []byte("the quick brown fox é 日本 ")[:1+r.Intn(20)]...)
		case 2:
			b = append(b, 0x00, 0xff)
		default:
			b = append(b, byte(r.Intn(256)))
		}
	}
	return b[:n]
}

// bxShort is 0 to 8 random bytes, for a second encoding joined to the first.
func bxShort(r *rand.Rand) []byte {
	d := bxData(r)
	return d[:min(len(d), r.Intn(9))]
}

// bxInsert puts s at a random byte offset of t (texts are ASCII until a
// multi-byte replacement, which comes last).
func bxInsert(r *rand.Rand, t, s string) string {
	k := r.Intn(len(t) + 1)
	return t[:k] + s + t[k:]
}

// baseText is a text near enc, an encoding over alphabet (padded or
// not); other is bytes of the other alphabets and near misses; next is a
// second encoding to join. The result is valid UTF-8.
func baseText(r *rand.Rand, enc, alphabet, other, next string, padded bool) string {
	switch r.Intn(12) {
	case 0, 1:
		return enc
	case 2: // newlines, which Go's decoders skip
		t := enc
		for k := 1 + r.Intn(3); k > 0; k-- {
			t = bxInsert(r, t, []string{"\n", "\r", "\r\n"}[r.Intn(3)])
		}
		return t
	case 3: // cut
		if len(enc) == 0 {
			return "="
		}
		return enc[:len(enc)-1-r.Intn(min(3, len(enc)))]
	case 4: // one byte replaced
		if len(enc) == 0 {
			return string(other[r.Intn(len(other))])
		}
		k := r.Intn(len(enc))
		repl := []string{string(other[r.Intn(len(other))]), "=", " ", "\t", "\x00", ".", "é", "日", "ÿ"}[r.Intn(9)]
		return enc[:k] + repl + enc[k+1:]
	case 5: // padding dropped, added or doubled
		if padded && strings.HasSuffix(enc, "=") {
			return strings.TrimRight(enc, "=")
		}
		return enc + []string{"=", "==", "==="}[r.Intn(3)]
	case 6: // two encodings joined
		return enc + next
	case 7: // a random string over the alphabet, with padding and newlines now and then
		n := r.Intn(41)
		var b strings.Builder
		for b.Len() < n {
			switch r.Intn(20) {
			case 0:
				b.WriteByte('=')
			case 1:
				b.WriteByte('\n')
			default:
				b.WriteByte(alphabet[r.Intn(len(alphabet))])
			}
		}
		return b.String()
	case 8: // padding or newlines alone
		return []string{"", "\n", "\r\n\r\n", "=", "==", "====", "\n=\n"}[r.Intn(7)]
	case 9: // `=` put in
		return bxInsert(r, enc, "=")
	case 10: // a symbol added
		return enc + string(alphabet[r.Intn(len(alphabet))])
	default: // a newline before or after the padding, and at the ends
		t := enc
		if r.Intn(2) == 0 {
			t = "\r\n" + t
		}
		if k := strings.IndexByte(t, '='); k >= 0 && r.Intn(2) == 0 {
			t = t[:k] + "\n" + t[k:]
		}
		return t + "\n"
	}
}

// bxAnswer is Go's DecodeString answer as the Xo check writes it.
func bxAnswer(b []byte, err error) string {
	if err != nil {
		return "err " + err.Error()
	}
	return "ok " + hex.EncodeToString(b)
}

// b64Encodings are std/base64's VARIANTS in order.
var b64Encodings = []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding}

func b64Cases(r *rand.Rand, n int) []bxCase {
	const std = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	const url = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	cs := make([]bxCase, 0, n)
	for len(cs) < n {
		v := r.Intn(4)
		e := b64Encodings[v]
		d := bxData(r)
		c := bxCase{variant: v, data: d, enc: e.EncodeToString(d)}
		alphabet, other := std, "-_.,:!*~"
		if v == 1 || v == 3 {
			alphabet, other = url, "+/.,:!*~"
		}
		c.text = baseText(r, c.enc, alphabet, other, e.EncodeToString(bxShort(r)), v < 2)
		c.want = bxAnswer(e.DecodeString(c.text))
		cs = append(cs, c)
	}
	return cs
}

const b64CheckXo = `use std/base64

const VARIANTS: List[base64.Base64] = [base64.Base64.Std, base64.Base64.Url, base64.Base64.RawStd, base64.Base64.RawUrl]

fn answer(r: Result[Bytes, base64.DecodeErr]) -> Str {
  match r {
    Ok(b) => "ok ${b.to_hex()}"
    Err(e) => "err ${e.display()}"
  }
}

test "std/base64 encodes as Go's EncodeToString" {
  var bad: List[Str] = []
  for (v, h, enc, _, _) in CASES {
    let d = Bytes.from_hex(h).or_fault("hex")
    if base64.encode(d, VARIANTS[v]) != enc || answer(base64.decode(enc, VARIANTS[v])) != "ok ${h}" || (v == 0 && d.to_base64() != enc) {
      bad.push("${v} ${h}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${CASES.len()} inputs encode differently, first (variant, bytes): ${bad[0]}")
  }
}

test "std/base64 decodes as Go's DecodeString" {
  var bad: List[Str] = []
  for (v, _, _, t, want) in CASES {
    let s = Bytes.from_hex(t).or_fault("hex").to_str() ?? ""
    let got = answer(base64.decode(s, VARIANTS[v]))
    var same = got == want
    if v == 0 {
      if let Some(b) = Bytes.from_base64(s) {
        same = same && "ok ${b.to_hex()}" == want
      }
    }
    if !same {
      bad.push("${v} ${t}: ${got}, Go ${want}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${CASES.len()} texts decode differently, first (variant, text hex): ${bad[0]}")
  }
}
`

// bxXoFile is the generated test file of the base32 or base64 harness.
func bxXoFile(name, goPkg, check string, seed int64, cases []bxCase) string {
	var b strings.Builder
	upper := strings.ToUpper(name)
	fmt.Fprintf(&b, "// Generated by internal/stdtest/%sfuzz_test.go (seed %d, %d cases) from Go's\n", name, seed, len(cases))
	fmt.Fprintf(&b, "// %s; do not edit: XO_%sFUZZ_WRITE=1 go test ./internal/stdtest -run Test%sFuzzCorpusCurrent\n", goPkg, upper, strings.ToUpper(name[:1])+name[1:])
	b.WriteString("// rewrites it.\n")
	b.WriteString(check)
	b.WriteString("\n/// (variant, bytes hex, Go's encoding, text hex, Go's DecodeString answer: ok HEX or err TEXT)\nconst CASES: List[(Int, Str, Str, Str, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%d, %q, %q, %q, %q),\n", c.variant, hex.EncodeToString(c.data), c.enc, hex.EncodeToString([]byte(c.text)), c.want)
	}
	b.WriteString("]\n")
	return b.String()
}

// bxCoverage fails when the cases stop reaching every variant, empty and
// long inputs, and both answers.
func bxCoverage(t *testing.T, cases []bxCase) {
	t.Helper()
	var variants [4]int
	empty, long, ok, bad, nl := 0, 0, 0, 0, 0
	for _, c := range cases {
		variants[c.variant]++
		if len(c.data) == 0 {
			empty++
		}
		if len(c.data) > 100 {
			long++
		}
		if strings.HasPrefix(c.want, "ok ") {
			ok++
			if strings.ContainsAny(c.text, "\r\n") {
				nl++
			}
		} else {
			bad++
		}
	}
	n := len(cases)
	for v, k := range variants {
		if k < n/8 {
			t.Fatalf("coverage fell: variant %d has %d of %d cases", v, k, n)
		}
	}
	if empty < n/100 || long < n/10 || ok < n/4 || bad < n/4 || nl < n/50 {
		t.Fatalf("coverage fell: %d empty, %d long, %d decode, %d fail, %d decode with newlines, of %d", empty, long, ok, bad, nl, n)
	}
}

// bxCorpusCurrent checks (or with XO_<NAME>FUZZ_WRITE=1 writes) the
// committed seed 1 corpus.
func bxCorpusCurrent(t *testing.T, path, env, text string) {
	t.Helper()
	if os.Getenv(env) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's package or the generator changed): %s=1 go test ./internal/stdtest -run %s", path, env, t.Name())
	}
}

const b64Corpus = "testdata/corpus/base64fuzz/base64fuzz_test.xo"

// TestBase64FuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestBase64FuzzCorpusCurrent(t *testing.T) {
	cases := b64Cases(rand.New(rand.NewSource(1)), 500)
	bxCoverage(t, cases)
	bxCorpusCurrent(t, b64Corpus, "XO_BASE64FUZZ_WRITE", bxXoFile("base64", "encoding/base64", b64CheckXo, 1, cases))
}

// TestBase64Fuzz runs a fresh seed's cases.
func TestBase64Fuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the base64 fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_BASE64FUZZ_SEED", 2))
	n := envInt(t, "XO_BASE64FUZZ_N", 2000)
	cases := b64Cases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "base64fuzz_test.xo", bxXoFile("base64", "encoding/base64", b64CheckXo, seed, cases), envBackends("XO_BASE64FUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
