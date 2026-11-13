// A fuzz harness for std/json against Go's encoding/json (2026-11-02,
// decision 0461: a parser of untrusted input is Early on testing until it
// is fuzzed).
//
// The corpus under testdata/corpus/json (scripts/stdcorpus) reads streams
// of mostly valid values; this harness aims at the edges a hostile body
// reaches: numbers at and past the range of Int and Float and the forms
// JSON forbids (01, 1., .5, +1, NaN), every escape including lone and
// paired surrogates, raw control bytes, bytes that are not UTF-8 (a lone
// continuation, an overlong form, an encoded surrogate), whitespace JSON
// does not allow, duplicate keys, deep nesting, trailing data, and each of
// those cut short or with a byte changed.
//
// Each case is compared two ways, as Go answers them:
//
//   - decode: json.decode[json.Value] of the raw bytes against
//     json.Unmarshal into any (ok or error), and on ok the value, rendered
//     the same on both sides: object fields sorted (Go's map has no order;
//     both keep a duplicate key's last value), strings as hex of their
//     bytes (so U+FFFD for a bad byte or a lone surrogate compares
//     exactly), a number with no fraction or exponent that fits an Int as
//     that Int and any other as its Float bits (decision 0299's Int/Num
//     split; Go's json.Number read with UseNumber).
//   - compact: json.compact of the text against json.Compact (error or the
//     exact bytes), for the cases whose bytes are UTF-8 (compact takes a
//     Str).
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/jsonfuzz/jsonfuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestJsonFuzzCorpusCurrent checks it is what this
// generator writes (XO_JSONFUZZ_WRITE=1 rewrites it). TestJsonFuzz runs a
// fresh seed (XO_JSONFUZZ_SEED, default 2; XO_JSONFUZZ_N cases, default
// 2000) on XO_JSONFUZZ_BACKENDS (default arm64 and llvm), for longer runs.
package stdtest

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// jfCase is one raw input and Go's answers for it.
type jfCase struct {
	raw []byte
	// decode is "err" or "ok RENDER"; compact is "err", "ok TEXT-AS-HEX",
	// or "" when raw is not UTF-8 (no compact case).
	decode, compact string
}

var jfNumbers = []string{
	"0", "-0", "1", "-1", "12", "0.5", "-0.25E-2", "1e3", "1E+2", "1e-2", "3.5", "100",
	"123456789012", "9007199254740993", "9223372036854775807", "9223372036854775808",
	"-9223372036854775808", "-9223372036854775809", "18446744073709551616",
	"1e400", "-1e400", "1e-400", "4.9e-324", "5e-324", "2.2250738585072014e-308",
	"1.7976931348623157e308", "1.7976931348623159e308", "0.1", "1.0", "-0.0", "0e0", "0E-0",
}

// jfBadNumbers are not JSON.
var jfBadNumbers = []string{
	"01", "1.", "-", ".5", "+1", "0x10", "1e", "1e+", "--1", "NaN", "Infinity", "-Infinity", "1_0", "00", "-01", "1.e2",
}

var jfStrPieces = []string{
	"a", "Z", " ", "\u00e9", "\u65e5", "\U0001F600", "\x5cn", "\x5c\"", "\x5c\x5c", "\x5c/", "\x5cb", "\x5cf", "\x5cr", "\x5ct", "\x5cu0041", "\x5cu00e9", "\x5cu0000", "\x5cu001f", "\x5cu007f", "\x5cu2028", "\x5cud800", "\x5cudc00", "\x5cud83d\x5cude00", "\x5cud800\x5cu0041", "\x5cuD834\x5cuDD1E", "\x5cudbff\x5cudfff", "\x5cud800\x5cud800", "\xe2\x80\xa8", "\x7f",
}

// jfBadStrPieces are escapes JSON does not have, raw bytes it does not
// allow in a string, and bytes that are not UTF-8 (Go reads those as
// U+FFFD). A backslash is spelled \x5c here.
var jfBadStrPieces = []string{
	"\x5cu12", "\x5cu12g4", "\x5cx41", "\x5c'", "\x5ca", "\t", "\x01", "\x1f", "\x00", "\xff", "\xc3", "\xc0\xaf", "\xed\xa0\x80", "\xf4\x90\x80\x80",
}

var jfKeywords = []string{"true", "false", "null"}
var jfBadKeywords = []string{"tru", "nul", "True", "NULL", "fals", "nulll", "t"}

var jfSpace = []string{"", "", "", " ", "\t", "\n", "\r\n", "  "}
var jfBadSpace = []string{"\x0b", "\x0c", "\xc2\xa0"}

func jfPick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

// jfMix picks from good, and from bad one time in n: most of a value stays
// valid, so the cases reach past the first error.
func jfMix(r *rand.Rand, good, bad []string, n int) string {
	if r.Intn(n) == 0 {
		return jfPick(r, bad)
	}
	return jfPick(r, good)
}

// jfString is a quoted string of 0 to 5 pieces; one in forty is left
// open.
func jfString(r *rand.Rand) string {
	var b strings.Builder
	b.WriteByte('"')
	for i, n := 0, r.Intn(6); i < n; i++ {
		b.WriteString(jfMix(r, jfStrPieces, jfBadStrPieces, 12))
	}
	if r.Intn(40) != 0 {
		b.WriteByte('"')
	}
	return b.String()
}

// jfValue is a value of up to depth levels of lists and objects.
func jfValue(r *rand.Rand, depth int) string {
	k := r.Intn(10)
	if depth <= 0 && k >= 6 {
		k = r.Intn(6)
	}
	switch {
	case k < 2:
		return jfMix(r, jfNumbers, jfBadNumbers, 8)
	case k < 4:
		return jfString(r)
	case k < 6:
		return jfMix(r, jfKeywords, jfBadKeywords, 10)
	case k < 8:
		var b strings.Builder
		b.WriteString("[" + jfMix(r, jfSpace, jfBadSpace, 30))
		for i, n := 0, r.Intn(4); i < n; i++ {
			if i > 0 {
				b.WriteString(jfMix(r, jfSpace, jfBadSpace, 30) + "," + jfMix(r, jfSpace, jfBadSpace, 30))
			}
			b.WriteString(jfValue(r, depth-1))
		}
		if r.Intn(40) == 0 {
			b.WriteString(",")
		}
		b.WriteString(jfMix(r, jfSpace, jfBadSpace, 30) + "]")
		return b.String()
	default:
		var b strings.Builder
		b.WriteString("{" + jfMix(r, jfSpace, jfBadSpace, 30))
		keys := []string{`"a"`, `"b"`, `"a"`, `""`, "\"\x5cu0061\""}
		for i, n := 0, r.Intn(4); i < n; i++ {
			if i > 0 {
				b.WriteString(jfMix(r, jfSpace, jfBadSpace, 30) + "," + jfMix(r, jfSpace, jfBadSpace, 30))
			}
			if r.Intn(2) == 0 {
				b.WriteString(jfPick(r, keys))
			} else {
				b.WriteString(jfString(r))
			}
			b.WriteString(jfMix(r, jfSpace, jfBadSpace, 30) + ":" + jfMix(r, jfSpace, jfBadSpace, 30))
			b.WriteString(jfValue(r, depth-1))
		}
		if r.Intn(40) == 0 {
			b.WriteString(",")
		}
		b.WriteString(jfMix(r, jfSpace, jfBadSpace, 30) + "}")
		return b.String()
	}
}

// jfCases builds the seeded cases: a value with space around it, then one
// in ten cut short, one in ten with a byte changed, one in sixteen with
// something after the value, and one in forty nested 30 to 120 deep.
func jfCases(r *rand.Rand, n int) []jfCase {
	cases := make([]jfCase, 0, n+4)
	for i := 0; i < n; i++ {
		s := jfMix(r, jfSpace, jfBadSpace, 30) + jfValue(r, 4) + jfMix(r, jfSpace, jfBadSpace, 30)
		if r.Intn(40) == 0 {
			d := 30 + r.Intn(91)
			open, shut := "[", "]"
			if r.Intn(2) == 0 {
				open, shut = `{"k":`, "}"
			}
			s = strings.Repeat(open, d) + jfValue(r, 1) + strings.Repeat(shut, d)
		}
		raw := []byte(s)
		switch r.Intn(10) {
		case 0:
			if len(raw) > 0 {
				raw = raw[:r.Intn(len(raw))]
			}
		case 1:
			if len(raw) > 0 {
				mutate := "[]{}\",:\\ 0e-.\xff"
				raw[r.Intn(len(raw))] = mutate[r.Intn(len(mutate))]
			}
		}
		if r.Intn(16) == 0 {
			raw = append(raw, jfPick(r, []string{"x", "]", " 1", "\x00", "{}", " "})...)
		}
		cases = append(cases, jfCase{raw: raw})
	}
	// Go's nesting limit is 10000 levels
	for _, d := range []int{9999, 10000, 10001} {
		cases = append(cases, jfCase{raw: []byte(strings.Repeat("[", d) + strings.Repeat("]", d))})
	}
	for i := range cases {
		cases[i].decode, cases[i].compact = jfGo(cases[i].raw)
	}
	return cases
}

// jfGo is Go's answer for raw.
func jfGo(raw []byte) (string, string) {
	decode := "err"
	var plain any
	if json.Unmarshal(raw, &plain) == nil {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		var v any
		if err := d.Decode(&v); err != nil {
			panic(fmt.Sprintf("%q: Unmarshal took it, UseNumber did not: %v", raw, err))
		}
		decode = "ok " + jfRender(v)
	}
	compact := ""
	if utf8.Valid(raw) {
		var b bytes.Buffer
		if err := json.Compact(&b, raw); err != nil {
			compact = "err"
		} else {
			compact = "ok " + fmt.Sprintf("%x", b.Bytes())
		}
	}
	return decode, compact
}

// jfRender is a decoded value in the shape the generated test's jf_r
// writes.
func jfRender(v any) string {
	switch v := v.(type) {
	case nil:
		return "n"
	case bool:
		if v {
			return "t"
		}
		return "f"
	case json.Number:
		t := string(v)
		// "-0" stays a Num (negative zero), as json.Value keeps it
		if !strings.ContainsAny(t, ".eE") && t != "-0" {
			if i, err := strconv.ParseInt(t, 10, 64); err == nil {
				return "i" + strconv.FormatInt(i, 10)
			}
		}
		f, _ := strconv.ParseFloat(t, 64)
		return "d" + strconv.FormatInt(int64(math.Float64bits(f)), 10)
	case string:
		return fmt.Sprintf("s%x", v)
	case []any:
		parts := make([]string, len(v))
		for i, x := range v {
			parts[i] = jfRender(x)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case map[string]any:
		parts := make([]string, 0, len(v))
		for k, x := range v {
			parts = append(parts, fmt.Sprintf("%x:%s", k, jfRender(x)))
		}
		sort.Strings(parts)
		return "{" + strings.Join(parts, ",") + "}"
	}
	panic(fmt.Sprintf("jfRender: %T", v))
}

const jfRenderXo = `use std/json

fn jf_r(v: json.Value) -> Str {
  match v {
    Null => "n"
    Bool(b) => if b { "t" } else { "f" }
    Int(n) => "i${n}"
    Num(f) => "d${f.to_bits()}"
    Str(s) => "s${s.bytes().to_hex()}"
    List(items) => {
      var parts: List[Str] = []
      for x in items {
        parts.push(jf_r(x))
      }
      "[" + parts.join(",") + "]"
    }
    Object(fields) => {
      var parts: List[Str] = []
      for (k, x) in fields {
        parts.push("${k.bytes().to_hex()}:${jf_r(x)}")
      }
      "{" + parts.sort().join(",") + "}"
    }
    _ => "trusted"
  }
}

fn jf_decode(raw: Bytes) -> Str {
  match json.decode[json.Value](raw) {
    Ok(v) => "ok " + jf_r(v)
    Err(_) => "err"
  }
}

fn jf_compact(text: Str) -> Str {
  match json.compact(text) {
    Ok(s) => "ok " + s.bytes().to_hex()
    Err(_) => "err"
  }
}

`

// jfXoFile is the generated test file: the renderer and one test per case.
func jfXoFile(seed int64, cases []jfCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/jsonfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// encoding/json; do not edit: XO_JSONFUZZ_WRITE=1 go test ./internal/stdtest -run TestJsonFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(jfRenderXo)
	for i, c := range cases {
		fmt.Fprintf(&b, "test \"json fuzz %d\" {\n  let got = jf_decode(%s)\n  if got != %s {\n    fault(\"decode: got \" + got)\n  }\n", i, ufBytes(c.raw), xoStr(c.decode))
		if c.compact != "" {
			fmt.Fprintf(&b, "  let c = jf_compact(%s)\n  if c != %s {\n    fault(\"compact: got \" + c)\n  }\n", xoStr(string(c.raw)), xoStr(c.compact))
		}
		b.WriteString("}\n\n")
	}
	b.WriteString(jfTypedXo)
	for i, in := range jfTyped {
		rec, one, m := jfTypedGo([]byte(in))
		fmt.Fprintf(&b, "test \"json typed %d\" {\n  let raw = %s\n", i, ufBytes([]byte(in)))
		fmt.Fprintf(&b, "  expect(jf_rec(raw)) == %s\n  expect(jf_one(raw)) == %s\n  expect(jf_map(raw)) == %s\n}\n\n", xoStr(rec), xoStr(one), xoStr(m))
	}
	// typed fields whose text other parsers read: Time (RFC 3339), Bytes
	// (base64), Duration (its text, as Go's ParseDuration reads it), one
	// record for every 10 cases, from a stream of the seed's own
	b.WriteString(jfFieldsXo)
	r := rand.New(rand.NewSource(seed*7919 + 3))
	for i := 0; i < (len(cases)+9)/10; i++ {
		in := jfFieldsText(r)
		fmt.Fprintf(&b, "test \"json fields %d\" {\n  expect(jf_fields(%s)) == %s\n}\n\n", i, ufBytes([]byte(in)), xoStr(jfFieldsGo([]byte(in))))
	}
	return b.String()
}

// jfTyped are inputs for typed decode, most with a duplicate key: Go's
// decoder reads every member, so a value a later duplicate replaces still
// fails the decode when it does not fit (decode[T] dropped those errors
// until 2026-11-02), and a replaced value that fits leaves the last one.
var jfTyped = []string{
	`{"a": 1, "a": 2}`, `{"a": "x", "a": 1}`, `{"a": 1, "a": "x"}`, `{"a": 1.5, "a": 1}`,
	`{"b": 1e400, "b": 1}`, `{"b": 1, "b": 1e400}`, `{"s": 1, "s": "t"}`, `{"s": "t", "s": null}`,
	`{"a": null, "a": 3}`, `{"a": 1, "b": 2.5, "s": "x", "a": 4}`, `{"z": "x", "z": 1, "a": 1}`,
	`{"a": [1], "a": 1}`, `{"a": {}, "a": 1}`, `{"b": true, "b": 0.5}`, `{"a": 9223372036854775808, "a": 1}`,
	`{"a": 1, "b": 2, "a": 3, "b": "q"}`, `{"\u0061": "x", "a": 1}`, `{"a": 1}`, `{}`,
}

// jfTypedGo is Go's answer for in decoded into a struct of three optional
// fields, a struct of one, and a map of ints: "err" or the fields.
func jfTypedGo(in []byte) (string, string, string) {
	var rec struct {
		A *int64   `json:"a"`
		B *float64 `json:"b"`
		S *string  `json:"s"`
	}
	r := "err"
	if json.Unmarshal(in, &rec) == nil {
		r = "a=" + jfOpt(rec.A) + "|b=" + jfOptF(rec.B) + "|s=" + jfOptS(rec.S)
	}
	var one struct {
		A *int64 `json:"a"`
	}
	o := "err"
	if json.Unmarshal(in, &one) == nil {
		o = "a=" + jfOpt(one.A)
	}
	var m map[string]int64
	ms := "err"
	if json.Unmarshal(in, &m) == nil {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := []string{}
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
		}
		ms = strings.Join(parts, ",")
	}
	return r, o, ms
}

func jfOpt(p *int64) string {
	if p == nil {
		return "none"
	}
	return strconv.FormatInt(*p, 10)
}

func jfOptF(p *float64) string {
	if p == nil {
		return "none"
	}
	return strconv.FormatInt(int64(math.Float64bits(*p)), 10)
}

func jfOptS(p *string) string {
	if p == nil {
		return "none"
	}
	return *p
}

const jfTypedXo = `struct JfRec {
  a: Int?
  b: Float?
  s: Str?
} derive Json

struct JfOne {
  a: Int?
} derive Json

fn jf_oi(o: Int?) -> Str {
  match o {
    Some(n) => "${n}"
    None => "none"
  }
}

fn jf_rec(raw: Bytes) -> Str {
  match json.decode[JfRec](raw) {
    Ok(r) => {
      let b = match r.b {
        Some(f) => "${f.to_bits()}"
        None => "none"
      }
      "a=${jf_oi(r.a)}|b=${b}|s=${r.s ?? "none"}"
    }
    Err(_) => "err"
  }
}

fn jf_one(raw: Bytes) -> Str {
  match json.decode[JfOne](raw) {
    Ok(r) => "a=${jf_oi(r.a)}"
    Err(_) => "err"
  }
}

fn jf_map(raw: Bytes) -> Str {
  match json.decode[Map[Str, Int]](raw) {
    Ok(m) => {
      var parts: List[Str] = []
      for (k, v) in m {
        parts.push("${k}=${v}")
      }
      parts.sort().join(",")
    }
    Err(_) => "err"
  }
}

`

// jfCoverage fails when the cases stop reaching the parser's parts: the
// floors are a fraction of what seed 1 reaches, so a generator change that
// loses a shape fails here.
func jfCoverage(t *testing.T, cases []jfCase) {
	t.Helper()
	oks, errs, ints, nums, strs, objs, compacts := 0, 0, 0, 0, 0, 0, 0
	for _, c := range cases {
		if c.decode == "err" {
			errs++
			continue
		}
		oks++
		if jfToken("i").MatchString(c.decode) {
			ints++
		}
		if jfToken("d").MatchString(c.decode) {
			nums++
		}
		if jfToken("s").MatchString(c.decode) {
			strs++
		}
		if strings.Contains(c.decode, "{") {
			objs++
		}
		if strings.HasPrefix(c.compact, "ok ") {
			compacts++
		}
	}
	t.Logf("answers: %d ok, %d err; ok with an Int %d, a Num %d, a Str %d, an object %d; compact ok %d", oks, errs, ints, nums, strs, objs, compacts)
	if oks < len(cases)/5 || errs < len(cases)/5 || ints < 50 || nums < 50 || strs < 50 || objs < 50 || compacts < len(cases)/5 {
		t.Fatalf("the cases stopped covering the parser: %d ok, %d err, %d Int, %d Num, %d Str, %d object, %d compact", oks, errs, ints, nums, strs, objs, compacts)
	}
}

// jfToken matches a rendered value of kind k at a value's start (after
// "ok ", '[', ',' or ':'; a hex key after ',' can look like one, which only
// loosens the floors).
func jfToken(k string) *regexp.Regexp {
	return regexp.MustCompile(`(^ok |[\[,:])` + k + `[-0-9a-f]`)
}

const jfCorpus = "testdata/corpus/jsonfuzz/jsonfuzz_test.xo"

// TestJsonFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestJsonFuzzCorpusCurrent(t *testing.T) {
	cases := jfCases(rand.New(rand.NewSource(1)), 2000)
	jfCoverage(t, cases)
	text := jfXoFile(1, cases)
	if os.Getenv("XO_JSONFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(jfCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(jfCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(jfCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's encoding/json or the generator changed): XO_JSONFUZZ_WRITE=1 go test ./internal/stdtest -run TestJsonFuzzCorpusCurrent", jfCorpus)
	}
}

// TestJsonFuzz runs a fresh seed's cases.
func TestJsonFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the json fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_JSONFUZZ_SEED", 2))
	n := envInt(t, "XO_JSONFUZZ_N", 2000)
	cases := jfCases(rand.New(rand.NewSource(seed)), n)
	for i, c := range cases {
		t.Logf("case %d %q", i, c.raw)
	}
	runFuzzDir(t, "jsonfuzz_test.xo", jfXoFile(seed, cases), envBackends("XO_JSONFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}

// runFuzzDir writes a generated test file of a public std module's cases
// into a temp directory and runs it with xo test on each backend, as
// TestStdCorpora runs a corpus: every generated test must pass.
func runFuzzDir(t *testing.T, file, content string, backends []string, cases int) {
	t.Helper()
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-fuzzdir-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	tests := testBlocks(content)
	if err := os.WriteFile(filepath.Join(work, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, b := range backends {
		cmd := testtmp.Guard(exec.Command(xo, "test", "--backend="+b, work))
		cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		out, err := cmd.CombinedOutput()
		passed, failed, ok := probeSummary(string(out))
		if err != nil || !ok || failed != 0 || passed != tests {
			t.Fatalf("xo test --backend=%s: %v; %d passed, %d failed; the generated file holds %d tests (%d generated cases)\n%s", b, err, passed, failed, tests, cases, out)
		}
		t.Logf("%s: %d passed of %d tests (%d generated cases)", b, passed, tests, cases)
	}
}

// jfFieldsText is a record of a Time, a Bytes and a Duration field, each
// valid text a Go value prints, or that text changed the way a hostile
// body would (a byte replaced, deleted, inserted, cut), or null, or left
// out.
func jfFieldsText(r *rand.Rand) string {
	field := func(valid string, alpha string) string {
		switch r.Intn(8) {
		case 0:
			return "null"
		case 1:
			return ""
		case 2, 3:
			b := []byte(valid)
			if len(b) > 0 {
				switch r.Intn(4) {
				case 0:
					b[r.Intn(len(b))] = alpha[r.Intn(len(alpha))]
				case 1:
					i := r.Intn(len(b))
					b = append(b[:i], b[i+1:]...)
				case 2:
					i := r.Intn(len(b) + 1)
					b = append(b[:i], append([]byte{alpha[r.Intn(len(alpha))]}, b[i:]...)...)
				default:
					b = b[:r.Intn(len(b))]
				}
			}
			return strconv.Quote(string(b))
		}
		return strconv.Quote(valid)
	}
	// 1700 to 2200 (inside what a Time holds, decision 0364), any
	// nanosecond, offsets of whole minutes up to 18 hours
	secs := r.Int63n(500*365*24*3600) - 270*365*24*3600
	t := time.Unix(secs, r.Int63n(1e9))
	offs := []int{0, 0, 3600, -18000, 19800, -34200, 45900, 86340, -86340}
	t = t.In(time.FixedZone("", offs[r.Intn(len(offs))]))
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z07:00"}
	when := t.Format(layouts[r.Intn(len(layouts))])
	raw := make([]byte, r.Intn(12))
	r.Read(raw)
	data := base64.StdEncoding.EncodeToString(raw)
	if r.Intn(6) == 0 {
		data = base64.RawStdEncoding.EncodeToString(raw)
	}
	durs := []string{time.Duration(r.Int63n(1 << 50)).String(), time.Duration(-r.Int63n(1 << 40)).String(), "1.5h", "300ms", "-2h45m", "1us", "2\u00b5s", "0", "1h1m1.000000001s", "9223372036854775807ns"}
	dur := durs[r.Intn(len(durs))]
	parts := []string{}
	for _, f := range []struct{ k, v string }{
		{"when", field(when, "0123456789:-+ZT.zt ")},
		{"data", field(data, "AZaz09+/=-_\n ")},
		{"dur", field(dur, "0123456789.hmsun-+ ")},
	} {
		if f.v != "" {
			parts = append(parts, strconv.Quote(f.k)+":"+f.v)
		}
	}
	r.Shuffle(len(parts), func(i, j int) { parts[i], parts[j] = parts[j], parts[i] })
	return "{" + strings.Join(parts, ",") + "}"
}

// jfFieldsGo is Go's answer for jfFieldsText's record: "err" or each field
// as the Xo side prints it (Unix nanoseconds, hex, nanoseconds; "none"
// for null or missing). Go has no Duration that decodes from text, so
// the field is a string run through time.ParseDuration, as Xo's decoder
// reads it; a Time outside what a Time holds (1677 to 2262) is an error
// in Xo (decision 0364).
func jfFieldsGo(in []byte) string {
	var rec struct {
		When *time.Time `json:"when"`
		Data []byte     `json:"data"`
		Dur  *string    `json:"dur"`
	}
	if json.Unmarshal(in, &rec) != nil {
		return "err"
	}
	w := "none"
	if rec.When != nil {
		if y := rec.When.UTC().Year(); y < 1678 || y > 2261 {
			return "err"
		}
		w = strconv.FormatInt(rec.When.UnixNano(), 10)
	}
	d := "none"
	if rec.Data != nil {
		d = fmt.Sprintf("%x", rec.Data)
	}
	u := "none"
	if rec.Dur != nil {
		x, err := time.ParseDuration(*rec.Dur)
		if err != nil {
			return "err"
		}
		u = strconv.FormatInt(int64(x), 10)
	}
	return "when=" + w + "|data=" + d + "|dur=" + u
}

const jfFieldsXo = `struct JfFields {
  when: Time?
  data: Bytes?
  dur: Duration?
} derive Json

fn jf_fields(raw: Bytes) -> Str {
  match json.decode[JfFields](raw) {
    Ok(r) => {
      let w = match r.when {
        Some(t) => "${t.unix_nanos()}"
        None => "none"
      }
      let d = match r.data {
        Some(b) => b.to_hex()
        None => "none"
      }
      let u = match r.dur {
        Some(x) => "${x.nanos()}"
        None => "none"
      }
      "when=${w}|data=${d}|dur=${u}"
    }
    Err(_) => "err"
  }
}

`
