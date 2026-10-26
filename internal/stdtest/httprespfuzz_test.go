// A differential of http1's response head parser against Go's
// net/http.ReadResponse on seeded raw response heads (2026-10-26).
//
// The client reads a server it does not trust, so parse_response (a private
// runtime function) sees hostile bytes: a status line with a bad code or
// version, an obs-folded field, a control byte in a value, a conflicting
// Content-Length, a Transfer-Encoding in other cases, a chunked body's
// Trailer. The test writes a generated http1_test.xo into a temp std copy
// (probestd_test.go) and runs one xo test per case, so a divergence is
// named by case number.
//
// The oracle is net/http.ReadResponse with method GET or HEAD. The shapes
// compared are the ones RespHead documents: status, clen (ReadResponse's
// ContentLength), chunked, close, and the header map sorted as name=value
// with U+001F between fields (a field's values joined by ", ", as Go joins
// them), after the port's documented drops (Transfer-Encoding, a chunked
// response's Content-Length and Trailer, a closing HTTP/1.1's Connection)
// and with Content-Length kept once and trimmed. The order of the fields is
// not compared: ReadResponse's map has none, and parse_response keeps the
// order received for the server's sake.
//
// A refused head is "bad" on both sides. The error text is not compared
// yet: RespHead.why says it is Go's text, which is its own row.
package stdtest

import (
	"bufio"
	"fmt"
	"math/rand"
	"net/http"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// rfCase is one raw response head, the request method that reads it, and
// the answer both sides must give.
type rfCase struct {
	head   string
	method string
	want   string
}

// rfVersion and rfCode are the status line's parts; the malformed ones are
// there on purpose.
var rfVersion = []string{
	"HTTP/1.1", "HTTP/1.1", "HTTP/1.1", "HTTP/1.0", "HTTP/1.2",
	"HTTP/2.0", "HTTP/1", "HTTP/1.10", "hTTP/1.1", "HTTP/1.1",
}

var rfCode = []string{
	"200", "204", "304", "100", "301", "404", "500", "599", "200", "200",
	"20", "2000", "+20", "-20", "2O0", " 200",
}

var rfReason = []string{
	"OK", "", "Not Found", "héllo 日本", "  spaced", "OK\tz",
}

// rfSpecial are the fields with readTransfer semantics; the rest of the
// block is drawn from hfField, which covers obs-folds, control bytes,
// repeated names and spaced names.
var rfSpecial = []string{
	"Content-Length: 0\r\n",
	"Content-Length: 5\r\n",
	"Content-Length:   5  \r\n",
	"Content-Length: 5\r\nContent-Length: 5\r\n",
	"Content-Length: 5\r\nContent-Length: 6\r\n",
	"Content-Length: abc\r\n",
	"Content-Length:\r\n",
	"Content-Length: -1\r\n",
	"Content-Length: +5\r\n",
	"Content-Length: 5, 5\r\n",
	"Transfer-Encoding: chunked\r\n",
	"Transfer-Encoding: Chunked\r\n",
	"Transfer-Encoding: gzip\r\n",
	"Transfer-Encoding: gzip, chunked\r\n",
	"Transfer-Encoding: chunked\r\nTransfer-Encoding: chunked\r\n",
	"Transfer-Encoding: chun\r\n ked\r\n",
	"Trailer: X-Foo\r\n",
	"Connection: close\r\n",
	"Connection: keep-alive\r\n",
	"Connection: Keep-Alive\r\n",
	"Pragma: no-cache\r\n",
	"Cache-Control: max-age=0\r\n",
	"Set-Cookie: a=b\r\n",
	"Set-Cookie: a=b\r\nSet-Cookie: c=d\r\n",
}

// rfCases builds n seeded response heads.
func rfCases(r *rand.Rand, n int) []rfCase {
	cases := make([]rfCase, 0, n)
	for i := 0; i < n; i++ {
		var b strings.Builder
		fmt.Fprintf(&b, "%s %s %s\r\n",
			rfVersion[r.Intn(len(rfVersion))],
			rfCode[r.Intn(len(rfCode))],
			rfReason[r.Intn(len(rfReason))])
		fields := r.Intn(6)
		for f := 0; f < fields; f++ {
			if r.Intn(2) == 0 {
				b.WriteString(rfSpecial[r.Intn(len(rfSpecial))])
				continue
			}
			hfField(r, &b)
		}
		if r.Intn(8) == 0 {
			b.WriteString("\n")
		} else {
			b.WriteString("\r\n")
		}
		method := "GET"
		if r.Intn(10) == 0 {
			method = "HEAD"
		}
		head := b.String()
		cases = append(cases, rfCase{head: head, method: method, want: rfGo(head, method)})
	}
	return cases
}

// rfGo is Go's answer for a raw head, in the shape rf_render writes on the
// Xo side (see the file comment for why each part is shaped this way).
func rfGo(head, method string) string {
	resp, err := http.ReadResponse(bufio.NewReader(strings.NewReader(head)), &http.Request{Method: method})
	if err != nil {
		return "bad"
	}
	h := resp.Header.Clone()
	// parse_response keeps one Content-Length, trimmed (fixLength)
	if cl, ok := h["Content-Length"]; ok {
		h.Del("Content-Length")
		h["Content-Length"] = []string{strings.Trim(cl[0], " \t")}
	}
	var keys []string
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+strings.Join(h[k], ", "))
	}
	chunked := len(resp.TransferEncoding) > 0 && resp.TransferEncoding[len(resp.TransferEncoding)-1] == "chunked"
	// RespHead.clen is readTransfer's realLength, not Go's public
	// ContentLength: HEAD, 1xx, 204 and 304 carry no body (0), chunked is
	// unbounded (-1), and only then is the header's length the answer.
	clen := resp.ContentLength
	switch {
	case method == "HEAD" || resp.StatusCode/100 == 1 || resp.StatusCode == 204 || resp.StatusCode == 304:
		clen = 0
	case chunked:
		clen = -1
	}
	// RespHead.close is net/http's shouldClose, not resp.Close: Go sets
	// resp.Close for an unbounded body too, and the port keeps that apart
	// (Exchange.keep). Go deletes Connection from the header when it closes,
	// so the tokens come from the raw field.
	conn := rfConnTokens(head)
	close := resp.ProtoMajor < 1 || rfHasToken(conn, "close") ||
		(resp.ProtoMajor == 1 && resp.ProtoMinor == 0 && !rfHasToken(conn, "keep-alive"))
	return fmt.Sprintf("%d|%d|%t|%t|%s", resp.StatusCode, clen, chunked, close, strings.Join(parts, "\x1f"))
}

// rfConnTokens lists the lower cased Connection tokens in the raw head's
// header block, read with textproto so an obs-folded Connection is one
// field with its continuation joined by a space (Go and the port see
// `close "q"`, not `close`).
func rfConnTokens(head string) []string {
	block := rfHeadOnly(head)
	i := strings.IndexByte(block, '\n')
	if i < 0 {
		return nil
	}
	mh, _ := textproto.NewReader(bufio.NewReader(strings.NewReader(block[i+1:]))).ReadMIMEHeader()
	var out []string
	for _, v := range mh["Connection"] {
		for _, p := range strings.Split(v, ",") {
			out = append(out, strings.ToLower(strings.TrimSpace(p)))
		}
	}
	return out
}

// rfHeadOnly is s up to and including the header block's terminating empty
// line (both readers end the head at the first "\n\n" or "\n\r\n").
func rfHeadOnly(s string) string {
	for i := 0; i+1 < len(s); i++ {
		if s[i] != '\n' {
			continue
		}
		if s[i+1] == '\n' {
			return s[:i+2]
		}
		if s[i+1] == '\r' && i+2 < len(s) && s[i+2] == '\n' {
			return s[:i+3]
		}
	}
	return s
}

// rfHasToken reports whether toks holds tok.
func rfHasToken(toks []string, tok string) bool {
	for _, t := range toks {
		if t == tok {
			return true
		}
	}
	return false
}

// rfXoFile is the generated test file: the renderer and one test per case.
func rfXoFile(cases []rfCase) string {
	var b strings.Builder
	b.WriteString("// Generated by TestHttpResponseFuzz; not part of the tree.\n\n")
	b.WriteString(`fn rf_render(p: RespHead, method: Str) -> Str {
  if p.res != OK {
    return "bad"
  }
  var acc: Map[Str, Str] = {}
  var i = 0
  while i + 1 < p.headers.len() {
    let k = p.headers[i]
    let v = p.headers[i + 1]
    let cur = match acc.get(k) {
      Some(x) => x + ", " + v
      None => v
    }
    acc.put(k, cur)
    i += 2
  }
  let hs = acc.keys().sort().map(fn(k) {
    let v = match acc.get(k) {
      Some(x) => x
      None => ""
    }
    k + "=" + v
  }).join("\u{1f}")
  "${p.status}|${p.clen}|${p.chunked}|${p.close}|${hs}"
}

`)
	for i, c := range cases {
		// the head as Go's %q text, plain ASCII, so a failure's message names
		// the exact bytes without a control character breaking the line
		fmt.Fprintf(&b, "test \"resp case %d\" {\n  let got = rf_render(parse_response(%s, true, %s), %s)\n  if got != %s {\n    fault(\"head \" + %s + \" got \" + got + \" want \" + %s)\n  }\n}\n\n",
			i, xoStr(c.head), xoStr(c.method), xoStr(c.method), xoStr(c.want), xoStr(strconv.Quote(c.head)), xoStr(c.want))
	}
	return b.String()
}

// TestHttpResponseFuzz runs the generated response heads with
// parse_response on the native backends and expects Go's
// net/http.ReadResponse answer for each. XO_RESPFUZZ_N sets the case count
// (default 300) and XO_RESPFUZZ_BACKENDS the backends (default arm64).
func TestHttpResponseFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the raw response block fuzz is a heavy test")
	}
	n := envInt(t, "XO_RESPFUZZ_N", 300)
	backends := envBackends("XO_RESPFUZZ_BACKENDS", []string{"arm64"})
	cases := rfCases(rand.New(rand.NewSource(65)), n)
	probeModule(t, "native/llrt/http1", "native/llrt/http1_test.xo", rfXoFile(cases), backends, len(cases))
}
