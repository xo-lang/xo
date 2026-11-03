// A fuzz harness for std/mime against Go's mime and net/http (2026-11-02,
// decision 0461: a parser of untrusted input is Early on testing until it
// is fuzzed).
//
// std/mime is Go's mime package (decision 0282): parse_media_type is
// mime.ParseMediaType, its parameter values that are not UTF-8 left out
// (a Str holds text), format_media_type is mime.FormatMediaType,
// detect_content_type is net/http's DetectContentType, and
// type_by_extension reads Go's builtin table only. The answers come from
// Go's packages, and from the Go backend's functions (std/rt/mime.go)
// where std's rule adds to Go's.
//
// Each case is one of: a Content-Type header built from pieces a hostile
// client sends (types and subtypes with and without case, missing halves,
// extra slashes, non-ASCII; parameters quoted and bare, with escapes,
// duplicated in either case, RFC 2231 charsets, languages and percent
// escapes, continuations in and out of order or with a gap, empty names
// and values, stray semicolons and spaces), cut short or with a byte
// changed; a type and parameters to format (names that are not tokens,
// values with spaces, quotes, backslashes, control bytes and non-ASCII);
// data to sniff (every signature DetectContentType knows, after leading
// whitespace for the HTML ones, then text or binary filler, cut at any
// length, sometimes past 512 bytes); or an extension in mixed case.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/mimefuzz/mimefuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestMimeFuzzCorpusCurrent checks it is what this
// generator writes (XO_MIMEFUZZ_WRITE=1 rewrites it). TestMimeFuzz runs a
// fresh seed (XO_MIMEFUZZ_SEED, default 2; XO_MIMEFUZZ_N cases, default
// 2000) on XO_MIMEFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"fmt"
	"math/rand"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xo-lang/xo/std/rt"
)

// mfCase is one call and Go's answer.
type mfCase struct {
	kind string // "parse", "format", "detect", "ext"
	text string
	// format's parameters, in order (a repeated name: the last wins)
	names, values []string
	data          []byte
	want          string
}

var mfTypes = []string{"text/html", "TEXT/Plain", "multipart/form-data", "application/json", "image/svg+xml", "x-y/z.w-v", "message/rfc822", "a/b/c", "", "text", "text/", "/html", "tëxt/html", "text /html", "text/ html", " text/html", "text/html ", "*/*", "form-data", "attachment", "inline"}

var mfParams = []string{
	"; charset=utf-8", ";charset=\"utf-8\"", "; charset=UTF-8", "; boundary=----x", "; boundary=\"a b\"",
	`; name="a\"b"`, `; name="a\\b"`, `; name="a\b"`, "; a=1; A=2", "; a=1; a=1", "; Q=v",
	"; title*=us-ascii'en-us'This%20is%20%2A%2A%2Afun%2A%2A%2A", "; title*=UTF-8''%e2%82%ac%20rates", "; title*=iso-8859-1''%a3",
	"; title*=utf-8''%zz", "; title*=''plain", "; title*=bad",
	"; t*0=\"a\"; t*1=\"b\"", "; t*0*=utf-8''%41; t*1=b", "; t*1=b", "; t*0=a; t*2=c", "; t*1=b; t*0=a", "; t*0=a; t=z",
	";", "; ", "; =x", "; a", "; a=", "; a=\"unterminated", "; a=b c", "; a = b", "; a=\"\"", "; a=é", "; é=a", "; a=\x01",
	"; filename=\"C:\\\\dir\\\\f.txt\"", "; filename=f.txt", "; filename*=UTF-8''%C3%A9.txt",
}

var mfNames = []string{"charset", "boundary", "name", "Title", "a", "a b", "é", "", "x-y", "a*", "a=b"}
var mfValues = []string{"utf-8", "a b", `a"b`, `a\b`, "é", "€ rates", "", "\x01", "tab\there", "plain", "line\nbreak", "semi;colon"}

// Signatures DetectContentType knows (net/http/sniff.go), some with the
// leading whitespace its HTML patterns skip.
var mfSigs = []string{
	"<!DOCTYPE HTML", " <!doctype html>", "\t\n<HTML", "<HEAD", "<SCRIPT", "<IFRAME", "<H1", "<DIV", "<FONT", "<TABLE", "<A", "<STYLE", "<TITLE", "<B", "<BODY", "<BR", "<P", "<!--", "<?xml", "%PDF-", "%!PS-Adobe-",
	"\xFE\xFF", "\xFF\xFE", "\xEF\xBB\xBF", "GIF87a", "GIF89a", "\x89PNG\x0D\x0A\x1A\x0A", "\xFF\xD8\xFF", "BM", "RIFF\x00\x00\x00\x00WEBPVP", "\x00\x00\x01\x00", "\x00\x00\x02\x00",
	"FORM\x00\x00\x00\x00AIFF", "ID3", "OggS\x00", "MThd\x00\x00\x00\x06", "RIFF\x00\x00\x00\x00AVI ", "RIFF\x00\x00\x00\x00WAVE", "\x00\x00\x00\x18ftypmp42", "\x00\x00\x00\x14ftyp", "\x1A\x45\xDF\xA3", "\x00\x00\x01\xBA", "\x00\x00\x01\xB3",
	"wOFF", "wOF2", "OTTO", "\x00\x01\x00\x00", "ttcf", "PK\x03\x04", "\x1F\x8B\x08", "Rar!\x1A\x07\x00", "Rar!\x1A\x07\x01\x00", "\x00asm", "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00LP",
	"", "plain text", "{\"json\": 1}",
}

var mfExts = []string{".png", ".PNG", ".Html", ".htm", ".js", ".mjs", ".json", ".css", ".svg", ".wasm", ".pdf", ".xml", ".avif", ".webp", ".unknown", "", "png", ".", ".JPEG", ".jpg", ".gif"}

func mfPick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

func mfHeader(r *rand.Rand) string {
	var b strings.Builder
	b.WriteString(mfPick(r, mfTypes))
	for i, n := 0, r.Intn(4); i < n; i++ {
		b.WriteString(mfPick(r, mfParams))
	}
	s := []byte(b.String())
	switch r.Intn(10) {
	case 0:
		s = s[:r.Intn(len(s)+1)]
	case 1:
		if len(s) > 0 {
			s[r.Intn(len(s))] = "; =\"'*%\\ /"[r.Intn(10)]
		}
	}
	return string(s)
}

// mfFiller is text or binary bytes after a signature.
func mfFiller(r *rand.Rand) []byte {
	n := r.Intn(40)
	if r.Intn(12) == 0 {
		n = 500 + r.Intn(40)
	}
	b := make([]byte, n)
	bin := r.Intn(3) == 0
	for i := range b {
		if bin {
			b[i] = byte(r.Intn(256))
		} else {
			b[i] = " >abc\n"[r.Intn(6)]
		}
	}
	return b
}

// mfParse is the answer for parse_media_type as the test renders it.
func mfParse(v string) string {
	res := rt.Mime_parse_media_type(nil, v)
	if !res.Ok {
		return "err " + res.E.F_msg
	}
	var keys []string
	for k := range rt.Map_All(res.V.F_params) {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var o strings.Builder
	fmt.Fprintf(&o, "ok %x", res.V.F_kind)
	for _, k := range keys {
		fmt.Fprintf(&o, ";%x=%x", k, rt.Map_get(nil, res.V.F_params, k).V)
	}
	return o.String()
}

func mfCases(r *rand.Rand, n int) []mfCase {
	cases := make([]mfCase, 0, n)
	for len(cases) < n {
		var c mfCase
		switch k := r.Intn(20); {
		case k < 10:
			c.kind, c.text = "parse", mfHeader(r)
			if !utf8.ValidString(c.text) {
				continue
			}
			c.want = mfParse(c.text)
		case k < 14:
			c.kind, c.text = "format", mfPick(r, mfTypes)
			m := map[string]string{}
			for i, n := 0, r.Intn(4); i < n; i++ {
				name, value := mfPick(r, mfNames), mfPick(r, mfValues)
				c.names, c.values = append(c.names, name), append(c.values, value)
				m[name] = value
			}
			c.want = mime.FormatMediaType(c.text, m)
		case k < 19:
			c.kind = "detect"
			c.data = append([]byte(mfPick(r, mfSigs)), mfFiller(r)...)
			if r.Intn(5) == 0 {
				c.data = c.data[:r.Intn(len(c.data)+1)]
			}
			c.want = http.DetectContentType(c.data)
		default:
			c.kind, c.text = "ext", mfPick(r, mfExts)
			c.want = "none"
			if o := rt.Mime_type_by_extension(nil, c.text); o.Some {
				c.want = "some " + o.V
			}
		}
		cases = append(cases, c)
	}
	return cases
}

const mfRenderXo = `use std/mime

fn mf_parse(v: Str) -> Str {
  match mime.parse_media_type(v) {
    Ok(m) => {
      var keys: List[Str] = []
      for (k, _) in m.params {
        keys.push(k)
      }
      var out = "ok " + m.kind.bytes().to_hex()
      for k in keys.sort() {
        out = out + ";" + k.bytes().to_hex() + "=" + (m.params.get(k) ?? "").bytes().to_hex()
      }
      out
    }
    Err(e) => "err " + e.display()
  }
}

fn mf_format(kind: Str, names: List[Str], values: List[Str]) -> Str {
  var m: Map[Str, Str] = {}
  for i in 0..names.len() {
    m.put(names[i], values[i])
  }
  mime.format_media_type(kind, m)
}

fn mf_ext(e: Str) -> Str {
  match mime.type_by_extension(e) {
    Some(t) => "some " + t
    None => "none"
  }
}

fn expect(got: Str, want: Str) {
  if got != want {
    fault("got " + got)
  }
}

`

func mfStrList(xs []string) string {
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = xoStr(x)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// mfXoFile is the generated test file: the helpers and one test per case.
func mfXoFile(seed int64, cases []mfCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/mimefuzz_test.go (seed %d, %d cases) from Go's mime\n", seed, len(cases))
	b.WriteString("// and net/http; do not edit: XO_MIMEFUZZ_WRITE=1 go test ./internal/stdtest -run TestMimeFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(mfRenderXo)
	for i, c := range cases {
		var call string
		switch c.kind {
		case "parse":
			call = fmt.Sprintf("mf_parse(%s)", xoStr(c.text))
		case "format":
			call = fmt.Sprintf("mf_format(%s, %s, %s)", xoStr(c.text), mfStrList(c.names), mfStrList(c.values))
		case "detect":
			call = fmt.Sprintf("mime.detect_content_type(%s)", ufBytes(c.data))
		default:
			call = fmt.Sprintf("mf_ext(%s)", xoStr(c.text))
		}
		fmt.Fprintf(&b, "test \"mime fuzz %d %s\" {\n  expect(%s, %s)\n}\n\n", i, c.kind, call, xoStr(c.want))
	}
	return b.String()
}

// mfCoverage fails when the cases stop reaching the functions' parts.
func mfCoverage(t *testing.T, cases []mfCase) {
	t.Helper()
	count := map[string]int{}
	answers := map[string]map[string]bool{}
	for _, c := range cases {
		count[c.kind]++
		w := c.want
		if c.kind == "parse" {
			if strings.HasPrefix(w, "ok ") {
				w = fmt.Sprintf("ok with %d params", strings.Count(w, ";"))
			}
		}
		if answers[c.kind] == nil {
			answers[c.kind] = map[string]bool{}
		}
		answers[c.kind][w] = true
	}
	t.Logf("cases %v; distinct answers: parse %d, format %d, detect %d, ext %d", count, len(answers["parse"]), len(answers["format"]), len(answers["detect"]), len(answers["ext"]))
	for w := range answers["parse"] {
		t.Logf("  parse: %s", w)
	}
	for w := range answers["detect"] {
		t.Logf("  detect: %s", w)
	}
	if len(answers["parse"]) < 8 || len(answers["format"]) < 100 || len(answers["detect"]) < 30 || len(answers["ext"]) < 8 {
		t.Fatalf("the cases stopped covering std/mime")
	}
}

const mfCorpus = "testdata/corpus/mimefuzz/mimefuzz_test.xo"

// TestMimeFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestMimeFuzzCorpusCurrent(t *testing.T) {
	cases := mfCases(rand.New(rand.NewSource(1)), 2000)
	mfCoverage(t, cases)
	text := mfXoFile(1, cases)
	if os.Getenv("XO_MIMEFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(mfCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mfCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(mfCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's mime or net/http or the generator changed): XO_MIMEFUZZ_WRITE=1 go test ./internal/stdtest -run TestMimeFuzzCorpusCurrent", mfCorpus)
	}
}

// TestMimeFuzz runs a fresh seed's cases.
func TestMimeFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the mime fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_MIMEFUZZ_SEED", 2))
	n := envInt(t, "XO_MIMEFUZZ_N", 2000)
	cases := mfCases(rand.New(rand.NewSource(seed)), n)
	for i, c := range cases {
		t.Logf("case %d %s %q %q", i, c.kind, c.text, c.data)
	}
	runFuzzDir(t, "mimefuzz_test.xo", mfXoFile(seed, cases), envBackends("XO_MIMEFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
