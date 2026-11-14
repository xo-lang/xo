// A fuzz harness for std/url against Go's net/url (decision 0461: a parser
// of untrusted input is fuzzed; decision 0616's fuzz kind). std/url is
// Go's query and path escaping and url.Values (spec/std.md 20); URL
// parsing itself is std/http's (httpurlfuzz_test.go).
//
// Each case is a random text from an alphabet of escapes (valid, cut,
// not hex, of bytes that are not UTF-8), `+`, `&`, `=`, `;`, spaces and
// multi-byte text, sometimes changed the way a hostile query would be (a
// byte replaced, deleted or inserted, or the text cut):
//
//   - query_escape and path_escape print what QueryEscape and PathEscape
//     print;
//   - query_unescape and path_unescape answer what QueryUnescape and
//     PathUnescape answer: the same error text, or the same text, where
//     a decoded byte that is not UTF-8 is kept as its `%XX` escape
//     (spec/std.md 20, decision 0259; the Go side applies that rule to
//     its bytes);
//   - parse_query gives ParseQuery's first error, or Values whose encode
//     is Encode of Go's Values under the same rule;
//   - a random sequence of add, set and remove on values() encodes as the
//     same sequence on url.Values encodes.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/urlfuzz/urlfuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestUrlFuzzCorpusCurrent checks it is what this
// generator writes (XO_URLQFUZZ_WRITE=1 rewrites it). TestUrlFuzz runs a
// fresh seed (XO_URLQFUZZ_SEED, default 2; XO_URLQFUZZ_N cases, default
// 2000) on XO_URLQFUZZ_BACKENDS (default arm64 and llvm). (XO_URLFUZZ is
// std/http's URL parser harness.)
package stdtest

import (
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// uqAlphabet is what a random text is built from: every byte class the
// escapers treat apart (unreserved, the path segment's extra keeps, the
// rest), escapes valid and broken, and query syntax.
var uqAlphabet = []string{
	"a", "Z", "0", "9", "-", ".", "_", "~", "$", "&", "+", ":", "=", "@",
	"/", "?", "#", ";", ",", " ", "%", "!", "*", "'", "(", ")", "\"", "\\",
	"\t", "\n", "\x00", "\x7f", "é", "日", "😀", "%20", "%41", "%2B", "%2b",
	"%25", "%ff", "%FF", "%C3", "%C3%A9", "%e6%97%a5", "%zz", "%2", "%g0",
	"%0", "a=1", "&b=", "=x", "k=v&k=w",
}

// uqMutBytes are the bytes a mutation puts in (ASCII, so the text stays
// UTF-8 for an Xo Str literal).
const uqMutBytes = "%&=+;a0F /z"

// uqText builds a text of 0 to 16 alphabet pieces, one in five then
// changed.
func uqText(r *rand.Rand) string {
	var b strings.Builder
	for i, n := 0, r.Intn(17); i < n; i++ {
		b.WriteString(uqAlphabet[r.Intn(len(uqAlphabet))])
	}
	s := b.String()
	if r.Intn(5) != 0 || s == "" {
		return s
	}
	bs := []byte(s)
	i := r.Intn(len(bs))
	switch r.Intn(4) {
	case 0:
		bs[i] = uqMutBytes[r.Intn(len(uqMutBytes))]
	case 1:
		bs = append(bs[:i], bs[i+1:]...)
	case 2:
		bs = append(bs[:i], append([]byte{uqMutBytes[r.Intn(len(uqMutBytes))]}, bs[i:]...)...)
	default:
		bs = bs[:i]
	}
	// a change may cut a rune: keep the case only when it is still UTF-8
	if !utf8.Valid(bs) {
		return s
	}
	return string(bs)
}

// uqTextOf is std/url's rule for decoded bytes: each byte of an invalid
// UTF-8 sequence as its %XX escape (upper case hex).
func uqTextOf(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			fmt.Fprintf(&b, "%%%02X", s[i])
		} else {
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}

// uqUnescape is Go's answer as the generated test renders it.
func uqUnescape(s string, query bool) string {
	var out string
	var err error
	if query {
		out, err = url.QueryUnescape(s)
	} else {
		out, err = url.PathUnescape(s)
	}
	if err != nil {
		return "err " + err.Error()
	}
	return "ok " + uqTextOf(out)
}

// uqQuery is ParseQuery's answer: "err TEXT" or "ok ENCODE", and false
// when two keys Go keeps apart become one text under uqTextOf (Xo merges
// them, and the order of their values is not Go's to give).
func uqQuery(s string) (string, bool) {
	v, err := url.ParseQuery(s)
	if err != nil {
		return "err " + err.Error(), true
	}
	m := url.Values{}
	for k, xs := range v {
		tk := uqTextOf(k)
		if _, dup := m[tk]; dup {
			return "", false
		}
		for _, x := range xs {
			m[tk] = append(m[tk], uqTextOf(x))
		}
	}
	return "ok " + m.Encode(), true
}

// uqOp is one edit of a Values: kind 0 add, 1 set, 2 remove.
type uqOp struct {
	kind       int
	key, value string
}

// uqKeys are few, so edits hit the same key often.
var uqKeys = []string{"a", "b", "a b", "é", "", "k=1", "%"}

func uqOps(r *rand.Rand) ([]uqOp, string) {
	var ops []uqOp
	v := url.Values{}
	for i, n := 0, r.Intn(9); i < n; i++ {
		o := uqOp{kind: r.Intn(5), key: uqKeys[r.Intn(len(uqKeys))]}
		if o.kind > 2 {
			o.kind = 0
		}
		if o.kind != 2 {
			o.value = uqText(r)
		}
		switch o.kind {
		case 0:
			v.Add(o.key, o.value)
		case 1:
			v.Set(o.key, o.value)
		default:
			v.Del(o.key)
		}
		ops = append(ops, o)
	}
	return ops, v.Encode()
}

// uqCase is one text with Go's answers, and one edit sequence.
type uqCase struct {
	text       string
	qesc, pesc string
	qun, pun   string
	query      string
	queryOK    bool
	ops        []uqOp
	opsEnc     string
}

func uqCases(r *rand.Rand, n int) []uqCase {
	var cs []uqCase
	for len(cs) < n {
		var c uqCase
		c.text = uqText(r)
		c.qesc = url.QueryEscape(c.text)
		c.pesc = url.PathEscape(c.text)
		c.qun = uqUnescape(c.text, true)
		c.pun = uqUnescape(c.text, false)
		c.query, c.queryOK = uqQuery(c.text)
		c.ops, c.opsEnc = uqOps(r)
		cs = append(cs, c)
	}
	return cs
}

const uqCheckXo = `use std/url

fn unescaped(r: Result[Str, url.EscapeErr]) -> Str {
  match r {
    Ok(s) => "ok ${s}"
    Err(e) => "err ${e.display()}"
  }
}

test "std/url escapes as Go's QueryEscape and PathEscape" {
  var bad: List[Str] = []
  for (s, q, p) in ESCAPES {
    let gq = url.query_escape(s)
    let gp = url.path_escape(s)
    if gq != q || gp != p {
      bad.push("${s.quote()}: query ${gq.quote()} want ${q.quote()}, path ${gp.quote()} want ${p.quote()}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${ESCAPES.len()} escapes differ, first: ${bad[0]}")
  }
}

test "std/url unescapes as Go's QueryUnescape and PathUnescape" {
  var bad: List[Str] = []
  for (s, q, p) in UNESCAPES {
    let gq = unescaped(url.query_unescape(s))
    let gp = unescaped(url.path_unescape(s))
    if gq != q || gp != p {
      bad.push("${s.quote()}: query ${gq.quote()} want ${q.quote()}, path ${gp.quote()} want ${p.quote()}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${UNESCAPES.len()} unescapes differ, first: ${bad[0]}")
  }
}

test "std/url parse_query answers as Go's ParseQuery and Encode" {
  var bad: List[Str] = []
  for (s, want) in QUERIES {
    let got = match url.parse_query(s) {
      Ok(v) => "ok ${v.encode()}"
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push("${s.quote()}: got ${got.quote()} want ${want.quote()}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${QUERIES.len()} queries differ, first: ${bad[0]}")
  }
}

test "std/url Values edits encode as Go's url.Values" {
  var bad: List[Str] = []
  for (ops, want) in EDITS {
    var v = url.values()
    for (kind, k, x) in ops {
      if kind == 0 {
        v.add(k, x)
      } else if kind == 1 {
        v.set(k, x)
      } else {
        v.remove(k)
      }
    }
    let got = v.encode()
    if got != want {
      bad.push("${ops.len()} edits: got ${got.quote()} want ${want.quote()}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${EDITS.len()} edit sequences differ, first: ${bad[0]}")
  }
}
`

// uqXoFile is the generated test file.
func uqXoFile(seed int64, cases []uqCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/urlfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// net/url; do not edit: XO_URLQFUZZ_WRITE=1 go test ./internal/stdtest -run TestUrlFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(uqCheckXo)
	b.WriteString("\n/// (text, QueryEscape, PathEscape)\nconst ESCAPES: List[(Str, Str, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%s, %s, %s),\n", xoStr(c.text), xoStr(c.qesc), xoStr(c.pesc))
	}
	b.WriteString("]\n\n/// (text, QueryUnescape, PathUnescape: `ok TEXT` or `err TEXT`)\nconst UNESCAPES: List[(Str, Str, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%s, %s, %s),\n", xoStr(c.text), xoStr(c.qun), xoStr(c.pun))
	}
	b.WriteString("]\n\n/// (text, ParseQuery: `ok ENCODE` or `err TEXT`)\nconst QUERIES: List[(Str, Str)] = [\n")
	for _, c := range cases {
		if c.queryOK {
			fmt.Fprintf(&b, "  (%s, %s),\n", xoStr(c.text), xoStr(c.query))
		}
	}
	b.WriteString("]\n\n/// (edits as (0 add, 1 set, 2 remove; key; value), Encode after them)\nconst EDITS: List[(List[(Int, Str, Str)], Str)] = [\n")
	for _, c := range cases {
		b.WriteString("  ([")
		for i, o := range c.ops {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "(%d, %s, %s)", o.kind, xoStr(o.key), xoStr(o.value))
		}
		fmt.Fprintf(&b, "], %s),\n", xoStr(c.opsEnc))
	}
	b.WriteString("]\n")
	return b.String()
}

// uqCoverage fails when the cases stop reaching the outcomes that matter.
func uqCoverage(t *testing.T, cases []uqCase) {
	t.Helper()
	qerr, qok, notUTF8, semi, qbad, qgood, multi := 0, 0, 0, 0, 0, 0, 0
	for _, c := range cases {
		if strings.HasPrefix(c.qun, "err") {
			qerr++
		} else {
			qok++
			if strings.Contains(c.qun, "%") && c.qun != "ok "+c.text {
				notUTF8++
			}
		}
		switch {
		case !c.queryOK:
		case strings.Contains(c.query, "semicolon"):
			semi++
		case strings.HasPrefix(c.query, "err"):
			qbad++
		case strings.Contains(c.query, "&"):
			multi++
			qgood++
		default:
			qgood++
		}
	}
	n := len(cases)
	if qerr < n/10 || qok < n/3 || notUTF8 < n/100 || semi < n/20 || qbad < n/20 || qgood < n/5 || multi < n/20 {
		t.Fatalf("coverage fell: unescape %d errors, %d ok (%d keeping invalid bytes as escapes); queries %d semicolon, %d other errors, %d ok (%d with several pairs) of %d", qerr, qok, notUTF8, semi, qbad, qgood, multi, n)
	}
}

const uqCorpus = "testdata/corpus/urlfuzz/urlfuzz_test.xo"

// TestUrlFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestUrlFuzzCorpusCurrent(t *testing.T) {
	cases := uqCases(rand.New(rand.NewSource(1)), 2000)
	uqCoverage(t, cases)
	text := uqXoFile(1, cases)
	if os.Getenv("XO_URLQFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(uqCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(uqCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(uqCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's net/url or the generator changed): XO_URLQFUZZ_WRITE=1 go test ./internal/stdtest -run TestUrlFuzzCorpusCurrent", uqCorpus)
	}
}

// TestUrlFuzz runs a fresh seed's cases.
func TestUrlFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the url fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_URLQFUZZ_SEED", 2))
	n := envInt(t, "XO_URLQFUZZ_N", 2000)
	cases := uqCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "urlfuzz_test.xo", uqXoFile(seed, cases), envBackends("XO_URLQFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
