// A differential of http1's request head parser against Go's
// net/http.ReadRequest on seeded raw header blocks (2026-10-26).
//
// The server's header block parser is http1.parse_head, a private runtime
// module (std/native/llrt/http1), so no corpus under internal/stdtest/
// testdata/corpus can reach it and the corpora never try an obs-fold line
// or a control byte where the field is read. This test copies std into a
// temp dir (the shape of stdprobe_test.go), writes a generated test file
// beside std/native/llrt/http1, and runs `xo test --std-module=
// native/llrt/http1` on it: each case is its own test, so a divergence is
// named by case number in the failure.
//
// The bytes on both sides are the text the parser sees: a Str, which holds
// valid UTF-8 only. The server's byte-to-text step (scan.text_at's U+FFFD,
// decision 0259) happens before parse_head and is covered by its own tests,
// so the alphabet carries U+FFFD rather than a raw invalid byte.
//
// The request line is fixed ("GET /x HTTP/1.1") and the generated fields
// leave out the names with their own semantics in head_of (Host,
// Content-Length, Transfer-Encoding, Connection, Set-Cookie); those are
// separate rows to add once the plain field block agrees.
package stdtest

import (
	"bufio"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// headCase is one raw request head and the answer both sides must give.
type headCase struct {
	head string
	want string
}

// hfNames are the field names the generator draws from. Every name is a
// token and none has semantics of its own in head_of.
var hfNames = []string{
	"X-Out", "X-In", "X-A", "X-B", "Accept", "Accept-Encoding",
	"User-Agent", "Content-Type", "Cookie", "X-Fold", "X-Empty", "X-Utf",
	"x-case", "X-0", "X-Hyphen-Name",
}

// hfValues are the values, written as the valid UTF-8 text the parser
// receives. Tabs, NUL, DEL, ESC and U+FFFD are there on purpose: the port's
// value scanner rejects every control byte but a tab, and Go's textproto
// reader is the question.
var hfValues = []string{
	"v", "", " ", "  spaced  ", "\tval", "val\t", "one, two", "a=b; c",
	"\"q\"", "héllo", "日本", "lo�ng", "\x7f", "\x00",
	"\x1b", "a\tb", "a b c", "x", "*/*", "gzip, deflate", `a"b`,
}

// hfHead builds raw request head cases from seed for n draws.
func hfHeadCases(r *rand.Rand, n int) []headCase {
	cases := make([]headCase, 0, n)
	for i := 0; i < n; i++ {
		var b strings.Builder
		b.WriteString("GET /x HTTP/1.1\r\n")
		hfHost(r, &b)
		// a field to hang a leader fold or an empty line on, so neither
		// lands on Host (a folded Host is a malformed Host to the server,
		// valid to ReadRequest: a layer difference, not a parser one)
		b.WriteString("X-Lead: lead\r\n")
		fields := r.Intn(6)
		for f := 0; f < fields; f++ {
			hfField(r, &b)
		}
		b.WriteString("\r\n")
		head := b.String()
		cases = append(cases, headCase{head: head, want: hfGo(head)})
	}
	return cases
}

// hfHost writes the Host field. The generated field block keeps a valid
// host: head_of is the server's parser and applies the server's rules to
// Host (400 "missing required Host header" without one, 400 "malformed Host
// header" for a bad one), which net/http's ReadRequest does not, so a
// malformed Host would compare two different layers. A Host table with the
// server's own rule is a separate row to add.
func hfHost(r *rand.Rand, b *strings.Builder) {
	b.WriteString("Host: example.com\r\n")
}

// hfField writes one field, its obs-fold continuations included.
func hfField(r *rand.Rand, b *strings.Builder) {
	name := hfNames[r.Intn(len(hfNames))]
	if r.Intn(6) == 0 {
		// a case variant of the same name, so repeated names merge as Go's
		// canonical map merges them
		name = strings.ToUpper(name[:1]) + strings.ToLower(name[1:])
	}
	value := hfValues[r.Intn(len(hfValues))]
	eol := "\r\n"
	if r.Intn(8) == 0 {
		eol = "\n"
	}
	switch r.Intn(12) {
	case 0:
		fmt.Fprintf(b, "%s:%s%s", name, value, eol)
	case 1:
		fmt.Fprintf(b, "%s: %s \t%s", name, value, eol)
	case 2:
		fmt.Fprintf(b, "%s : %s%s", name, value, eol)
	case 3:
		fmt.Fprintf(b, "%s\t: %s%s", name, value, eol)
	case 4:
		fmt.Fprintf(b, ": %s%s", value, eol)
	case 5:
		fmt.Fprintf(b, "NoColon %s%s", value, eol)
	case 6:
		fmt.Fprintf(b, "X Y: %s%s", value, eol)
	case 7:
		// an obs-fold continuation, spaces or a tab, one to three lines
		fmt.Fprintf(b, "%s: %s%s", name, value, eol)
		for k := 0; k < 1+r.Intn(3); k++ {
			lead := "  "
			if r.Intn(2) == 0 {
				lead = "\t"
			}
			fmt.Fprintf(b, "%s%s%s", lead, hfValues[r.Intn(len(hfValues))], eol)
		}
	case 8:
		// a fold with nothing before it
		fmt.Fprintf(b, "  %s%s", value, eol)
	case 9:
		fmt.Fprintf(b, "%s: %s\r\n", name, value)
		fmt.Fprintf(b, "%s: %s", name, hfValues[r.Intn(len(hfValues))])
		if r.Intn(2) == 0 {
			b.WriteString("\r\n")
		}
	case 10:
		b.WriteString("\r\n")
	case 11:
		fmt.Fprintf(b, "%s: %s, %s%s", name, value, hfValues[r.Intn(len(hfValues))], eol)
	}
}

// hfGo is Go's answer for a raw head, in the same shape hf_render writes on
// the Xo side: res ("bad" when either side refuses the head, whatever its
// code), method, target, major.minor, host, the header map sorted as
// name=value with U+001F between fields and Go's ", " between a field's
// values, then content length, chunked and close.
func hfGo(head string) string {
	req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(head)))
	if err != nil {
		return "bad"
	}
	// ReadRequest alone takes a name with a space; Go's server (and head_of,
	// which is that server's parser) answers 400 invalid header name for it
	// (httpguts.ValidHeaderFieldName: every byte a tchar), so the oracle
	// applies the server's rule.
	for k := range req.Header {
		if !hfToken(k) {
			return "bad"
		}
	}
	var keys []string
	for k := range req.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+strings.Join(req.Header[k], ", "))
	}
	return fmt.Sprintf("1|%s|%s|%d.%d|%s|%s|%d|%t|%t",
		req.Method, req.RequestURI, req.ProtoMajor, req.ProtoMinor, req.Host,
		strings.Join(parts, "\x1f"), req.ContentLength, len(req.TransferEncoding) > 0, req.Close)
}

// hfToken is httpguts.ValidHeaderFieldName: an RFC 9110 token, the bytes a
// header name may hold.
func hfToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c == '!', c == '#', c == '$', c == '%', c == '&', c == '\'',
			c == '*', c == '+', c == '-', c == '.', c == '^', c == '_',
			c == '`', c == '|', c == '~':
		default:
			return false
		}
	}
	return true
}

// hfXoFile is the test file written beside the http1 module: the renderer
// and one test per case, so a failure names the case.
func hfXoFile(cases []headCase) string {
	var b strings.Builder
	b.WriteString("// Generated by TestHttpHeadFuzz; not part of the tree.\n\n")
	b.WriteString(`fn hf_render(p: Parse) -> Str {
  if p.res != OK {
    return "bad"
  }
  let h = p.head
  var m: Map[Str, Str] = {}
  var i = 0
  while i + 1 < h.headers.len() {
    m.put(h.headers[i], h.headers[i + 1])
    i += 2
  }
  let hs = m.keys().sort().map(fn(k) {
    let v = match m.get(k) {
      Some(x) => x
      None => ""
    }
    k + "=" + v
  }).join("\u{1f}")
  "1|${h.method}|${h.target}|${h.major}.${h.minor}|${h.host}|${hs}|${h.clen}|${h.chunked}|${h.close}"
}

`)
	for i, c := range cases {
		fmt.Fprintf(&b, "test \"head case %d\" {\n  let got = hf_render(parse_head(%s, true))\n  if got != %s {\n    fault(\"got \" + got + \" want \" + %s)\n  }\n}\n\n",
			i, xoStr(c.head), xoStr(c.want), xoStr(c.want))
	}
	return b.String()
}

// xoStr is s as an Xo string literal.
func xoStr(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '$':
			b.WriteString(`\$`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u{%x}`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// TestHttpHeadFuzz runs the generated cases with parse_head on the native
// backends and expects Go's net/http.ReadRequest answer for each.
// XO_HEADFUZZ_N sets the case count (default 300) and XO_HEADFUZZ_BACKENDS
// the backends (default arm64).
func TestHttpHeadFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the raw header block fuzz is a heavy test")
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	n := 300
	if v := os.Getenv("XO_HEADFUZZ_N"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
			t.Fatalf("XO_HEADFUZZ_N: %v", err)
		}
	}
	backends := []string{"arm64"}
	if v := os.Getenv("XO_HEADFUZZ_BACKENDS"); v != "" {
		backends = strings.Split(v, ",")
	}
	cases := hfHeadCases(rand.New(rand.NewSource(64)), n)
	work, err := testtmp.Dir("xo-headfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	std := filepath.Join(work, "std")
	if err := os.CopyFS(std, os.DirFS(filepath.Join(root, "std"))); err != nil {
		t.Fatal(err)
	}
	tests := filepath.Join(std, "native", "llrt", "http1_test.xo")
	if err := os.WriteFile(tests, []byte(hfXoFile(cases)), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, b := range backends {
		cmd := testtmp.Guard(exec.Command(xo, "test", "--std-module=native/llrt/http1", "--backend="+b))
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "XO_STD="+std, "PWD="+root)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo test --std-module=native/llrt/http1 --backend=%s: %v\n%s", b, err, hfFailures(string(out)))
		}
	}
}

// hfFailures trims a failing xo test run to the lines that name cases.
func hfFailures(out string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "head case") || strings.Contains(line, "FAIL") || strings.Contains(line, "panic") || strings.Contains(line, "fault") || strings.Contains(line, "got ") {
			keep = append(keep, line)
		}
	}
	if len(keep) == 0 {
		return out
	}
	return strings.Join(keep, "\n")
}
