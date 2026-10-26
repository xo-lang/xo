// A differential of neturl.url_parse (net/url's Parse and ParseRequestURI)
// against Go's net/url on seeded URL strings (2026-10-26).
//
// neturl is a private runtime module (std/native/llrt/neturl), so no corpus
// under internal/stdtest/testdata/corpus can reach it. This test copies std
// into a temp dir (the shape of stdprobe_test.go), writes a generated test
// file beside std/native/llrt/neturl, and runs `xo test --std-module=
// native/llrt/neturl` on it: one test per case, so a divergence is named by
// case number.
//
// The input is Bytes, not a Str: a URL off the wire can hold bytes that are
// not UTF-8, and the parser is bytes based (rawb.str_slice, no conversion),
// so the generated test builds its Str with rt.cast[Bytes, Str] the way the
// callers do. Go's url.Parse takes the same bytes as a Go string, which may
// hold invalid UTF-8, so both sides see the same input.
//
// The fragment is cut before url_parse by every caller that parses a URL
// (http1 does it on the raw bytes), so the generated test cuts at the first
// '#' for Parse, while ParseRequestURI is given the bytes as they are: a
// request target carries no fragment (Go's doc) and http1 passes the target
// through. The two sides compare the same line, "err TEXT" or
// "ok SCHEME|HOST|PATH|RAWQUERY", as hex: a URL can put bytes into any of
// those fields that are not UTF-8, and hex compares the bytes each side
// actually holds, not a text conversion of them.
package stdtest

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// ufURL is one raw URL and the answers both parsers must give it.
type ufURL struct {
	raw []byte
	// want is "err TEXT" or "ok SCHEME|HOST|PATH|RAWQUERY", for Parse and
	// for ParseRequestURI.
	want, wantReq string
}

// ufPieces are the parts a structured URL is built from. Everything hostile
// a URL can carry is here: no scheme, a scheme an authority cannot follow, a
// userinfo with and without escapes, IPv6 literals with a port and a zone,
// percent escapes that are valid, truncated, or not hex, spaces, multi-byte
// text, raw invalid UTF-8, an empty host, a port that is not a number.
var ufPieces = [][]string{
	{"", "http", "https", "HTTP", "ftp", "a", "x+y", "1a", "mailto", "postgres", "\xff"},
	{"://", ":", ""},
	{"//", "/", "", "///"},
	{"", "user", "us:er", "%41", "a b", "u:p:q", "\xff", "u%zz"},
	{"example.com", "127.0.0.1", "[::1]", "[fe80::1%25eth0]", "h:80", "h:x", "[::1]:8080", "a..b", "é", "\xff", "", "h:", "h:999999999999999999999"},
	{"/", "/a/b", "/a%20b", "/%zz", "/a b", "/π", "/%2F", "/a%2", "/%ff", ""},
	{"?a=1", "?", "?a=%zz", "?é=1", "?x", "?a=1&b=2", ""},
	{"#f", "#", ""},
}

// ufAlphabet is the byte alphabet a fully random case draws from: URL syntax
// and the bytes that break it.
var ufAlphabet = []string{
	"a", "Z", "0", "/", ":", "?", "#", "@", "[", "]", "%", "2", "F", "z",
	".", "-", "~", "+", "&", "=", ";", " ", "(", ")", "\t", "\x00", "\x7f",
	"é", "日", "\xff", "\xc3", "%41", "%zz", "//", "http://", "https://", "mailto:",
}

// ufStructured builds one URL from the pieces, in order. Three cases in
// four carry an authority, so the host and port rules are reached as often
// as the path rules.
func ufStructured(r *rand.Rand) string {
	var b strings.Builder
	b.WriteString(ufPieces[0][r.Intn(len(ufPieces[0]))])
	b.WriteString(ufPieces[1][r.Intn(len(ufPieces[1]))])
	if r.Intn(4) != 0 {
		b.WriteString(ufPieces[2][r.Intn(len(ufPieces[2]))])
		if r.Intn(3) == 0 {
			b.WriteString(ufPieces[3][r.Intn(len(ufPieces[3]))])
			b.WriteString("@")
		}
		b.WriteString(ufPieces[4][r.Intn(len(ufPieces[4]))])
	}
	b.WriteString(ufPieces[5][r.Intn(len(ufPieces[5]))])
	b.WriteString(ufPieces[6][r.Intn(len(ufPieces[6]))])
	b.WriteString(ufPieces[7][r.Intn(len(ufPieces[7]))])
	return b.String()
}

// ufRandom builds a string of 0 to 12 alphabet pieces.
func ufRandom(r *rand.Rand) string {
	var b strings.Builder
	for i, n := 0, r.Intn(13); i < n; i++ {
		b.WriteString(ufAlphabet[r.Intn(len(ufAlphabet))])
	}
	return b.String()
}

// ufSchemes and ufHosts build the absolute shape: a scheme, an authority
// and then any tail. Every host rule has a row here, valid and not: an IPv4,
// IPv6 with and without a port and with a zone, a port that is a number, a
// port that is not, a name with an empty label, multi-byte text, a raw byte
// that is not UTF-8, an empty host, an escaped byte, and a userinfo with a
// password and with an escape.
var ufSchemes = []string{"http", "https", "HTTP", "ftp", "a"}
var ufHosts = []string{
	"example.com", "127.0.0.1", "[::1]", "[fe80::1%25eth0]", "h", "h:80", "h:x",
	"a..b", "é", "\xff", "", "h:", "h:999999999999999999999", "%41",
	"us:er@h", "u%41@h", "@h", "u@", "u@h:80", "u@[::1]:8080",
}

func ufAbsolute(r *rand.Rand) string {
	var b strings.Builder
	b.WriteString(ufSchemes[r.Intn(len(ufSchemes))])
	b.WriteString("://")
	b.WriteString(ufHosts[r.Intn(len(ufHosts))])
	b.WriteString(ufPieces[5][r.Intn(len(ufPieces[5]))])
	b.WriteString(ufPieces[6][r.Intn(len(ufPieces[6]))])
	b.WriteString(ufPieces[7][r.Intn(len(ufPieces[7]))])
	return b.String()
}

// ufGo is Go's answer for raw, in the shape the generated test renders: the
// line "err TEXT" or "ok SCHEME|HOST|PATH|RAWQUERY", as hex, because a URL
// can put bytes in any of those fields that are not UTF-8 and hex compares
// the bytes both sides hold. A Parse input is cut at the first '#' first:
// every caller cuts the fragment on the raw bytes before url_parse, and
// Go's Parse would otherwise validate escapes inside a fragment nobody
// reads. ParseRequestURI assumes the fragment was cut already (Go's doc: a
// request URL has no #fragment), and http1 reads an empty request target as
// "empty url" (the server's own error, not the library's), so both shapes
// follow those two rules.
func ufGo(raw []byte, viaRequest bool) string {
	if !viaRequest {
		if i := bytes.IndexByte(raw, '#'); i >= 0 {
			raw = raw[:i]
		}
	}
	s := string(raw)
	if viaRequest && len(raw) == 0 {
		return hex.EncodeToString([]byte("err empty url"))
	}
	var u *url.URL
	var err error
	if viaRequest {
		u, err = url.ParseRequestURI(s)
	} else {
		u, err = url.Parse(s)
	}
	line := ""
	if err != nil {
		if ue, ok := err.(*url.Error); ok {
			line = "err " + ue.Err.Error()
		} else {
			line = "err " + err.Error()
		}
	} else {
		line = "ok " + u.Scheme + "|" + ufText(u.Host) + "|" + ufText(u.Path) + "|" + u.RawQuery
	}
	return hex.EncodeToString([]byte(line))
}

// ufText is a component as neturl keeps it: a Str holds text, so a byte that
// is not part of valid UTF-8 stays its percent escape, uppercase hex, one
// byte at a time (decision 0259; scan.url_text and fix_utf8). Go's Path is
// the decoded bytes, invalid ones included, so the oracle applies the same
// rule before comparing.
func ufText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, "%%%02X", s[i])
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// ufCases builds the seeded cases: half structured, half random, a sixth of
// them cut short or with a byte changed so a truncated escape and a broken
// literal are covered.
func ufCases(r *rand.Rand, n int) []ufURL {
	cases := make([]ufURL, 0, n)
	for i := 0; i < n; i++ {
		var s string
		switch r.Intn(3) {
		case 0:
			s = ufStructured(r)
		case 1:
			s = ufAbsolute(r)
		default:
			s = ufRandom(r)
		}
		raw := []byte(s)
		switch r.Intn(6) {
		case 0:
			if len(raw) > 0 {
				raw = raw[:r.Intn(len(raw))]
			}
		case 1:
			if len(raw) > 0 {
				mutate := "/:?#@[]%a \xff"
				raw[r.Intn(len(raw))] = mutate[r.Intn(len(mutate))]
			}
		}
		cases = append(cases, ufURL{raw: raw, want: ufGo(raw, false), wantReq: ufGo(raw, true)})
	}
	return cases
}

// ufXoFile is the generated test file: the renderer and one test per case.
const ufRender = `use native/rt

fn uf_line(raw: Bytes, via: Bool) -> Str {
  // Parse assumes the caller cut the fragment (http1 does it on the raw
  // bytes); ParseRequestURI assumes a request target, which never carries one
  var n = raw.len()
  if !via {
    var i = 0
    while i < n && Int(raw[i]) != 35 {
      i += 1
    }
    n = i
  }
  let u = url_parse(rt.cast[Bytes, Str](raw[0..n]), via)
  if !u.ok {
    return "err ${u.err}"
  }
  "ok ${u.scheme}|${u.host}|${u.path}|${u.rawquery}"
}

`

func ufXoFile(cases []ufURL) string {
	var b strings.Builder
	b.WriteString("// Generated by TestHttpUrlFuzz; not part of the tree.\n\n")
	b.WriteString(ufRender)
	for i, c := range cases {
		fmt.Fprintf(&b, "test \"url case %d\" {\n  let raw = %s\n  let via_req = uf_line(raw, true).bytes().to_hex()\n  if via_req != %s {\n    fault(\"request uri: got \" + via_req + \" want \" + %s)\n  }\n  let got = uf_line(raw, false).bytes().to_hex()\n  if got != %s {\n    fault(\"parse: got \" + got + \" want \" + %s)\n  }\n}\n\n",
			i, ufBytes(c.raw), xoStr(c.wantReq), xoStr(c.wantReq), xoStr(c.want), xoStr(c.want))
	}
	return b.String()
}

// ufBytes is raw as an Xo Bytes literal, so a byte that is not UTF-8 reaches
// the parser unchanged.
func ufBytes(raw []byte) string {
	var o strings.Builder
	o.WriteString(`b"`)
	for _, c := range raw {
		if c >= 0x20 && c < 0x7f && c != '"' && c != '\\' && c != '$' {
			o.WriteByte(c)
		} else {
			fmt.Fprintf(&o, `\x%02x`, c)
		}
	}
	o.WriteByte('"')
	return o.String()
}

// ufCoverage fails when the cases stopped reaching the parser's parts: an
// error only table, or one with no scheme, host, path or query to compare,
// would pass without comparing the fields. The floors are a fraction of
// what the generator reaches today, so a changed seed that loses a shape
// fails here.
func ufCoverage(t *testing.T, cases []ufURL) {
	t.Helper()
	errs, schemes, hosts, paths, queries := 0, 0, 0, 0, 0
	seenScheme := map[string]bool{}
	for _, c := range cases {
		line, err := hex.DecodeString(c.want)
		if err != nil {
			t.Fatalf("case %q: bad want %q", c.raw, c.want)
		}
		if bytes.HasPrefix(line, []byte("err ")) {
			errs++
			continue
		}
		parts := strings.Split(string(line[len("ok "):]), "|")
		if len(parts) != 4 {
			t.Fatalf("case %q: want %q has %d fields", c.raw, line, len(parts))
		}
		if parts[0] != "" && !seenScheme[parts[0]] {
			seenScheme[parts[0]] = true
			schemes++
		}
		if parts[1] != "" {
			hosts++
		}
		if parts[2] != "" {
			paths++
		}
		if parts[3] != "" {
			queries++
		}
	}
	t.Logf("answers: %d err, %d schemes, %d with a host, %d with a path, %d with a query", errs, schemes, hosts, paths, queries)
	if errs < 300 || schemes < 4 || hosts < 100 || paths < 500 || queries < 300 {
		t.Fatalf("the cases stopped covering the parser: %d err, %d schemes, %d hosts, %d paths, %d queries", errs, schemes, hosts, paths, queries)
	}
}

// TestHttpUrlFuzz runs the generated cases with url_parse on arm64 and LLVM
// and expects Go's net/url answer for each. XO_URLFUZZ_N sets the case count
// (default 2000), XO_URLFUZZ_BACKENDS the backends.
func TestHttpUrlFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the URL parse fuzz is a heavy test")
	}
	n := envInt(t, "XO_URLFUZZ_N", 2000)
	backends := envBackends("XO_URLFUZZ_BACKENDS", []string{"arm64", "llvm"})
	cases := ufCases(rand.New(rand.NewSource(68)), n)
	ufCoverage(t, cases)
	for i, c := range cases {
		// the generated test names a failure by case number; this line is
		// that number's input
		t.Logf("case %d %q", i, c.raw)
	}
	probeModule(t, "native/llrt/neturl", filepath.Join("native", "llrt", "neturl_test.xo"), ufXoFile(cases), backends, len(cases))
}
