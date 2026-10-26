// A request smuggling table for http1's server side (2026-10-26).
//
// Both sides read the same raw bytes: a Go net/http server on 127.0.0.1
// answers every request it accepts, and the Xo side runs the private pair
// the server uses, parse_head and then chunked for a chunked body, in a
// generated http1_test.xo (probestd_test.go). The comparison is the
// framing, not only accept or reject: for each request either side sees,
// the framing kind (Content-Length or chunked), the length and the body
// bytes, joined in order, so the pipelined row also pins where the next
// request starts. Request smuggling is the case where both sides accept
// and then frame the body differently, which a verdict-only table cannot
// see.
//
// The Xo side takes the framing from parse_head's own result (Head.clen
// and Head.chunked) and reads the body that way; nothing in a row says
// which way to read it.
//
// The table is hand written, not seeded: each row is a smuggling shape
// named in the test, so a failure reads as the vector. The listener is
// 127.0.0.1 with the OS's port, and the test closes it.
package stdtest

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// smCase is one raw request.
type smCase struct {
	name string
	raw  string
}

// smBody is a chunked body: 5 bytes, then the last chunk.
const smBody = "5\r\nhello\r\n0\r\n\r\n"

// smCases are the smuggling shapes. A Content-Length is paired with a five
// byte body where the request is meant to be complete.
func smCases() []smCase {
	return []smCase{
		{"cl equal duplicates", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length: 5\r\nContent-Length: 5\r\n\r\nhello"},
		{"cl conflicting", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length: 5\r\nContent-Length: 6\r\n\r\nhello"},
		{"cl signed", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length: +5\r\n\r\nhello"},
		{"cl spaced", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length:   5  \r\n\r\nhello"},
		{"cl empty", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length:\r\n\r\n"},
		{"cl empty then five", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length:\r\nContent-Length: 5\r\n\r\nhello"},
		{"cl hex", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length: 0x5\r\n\r\n"},
		{"cl huge", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length: 999999999999999999999\r\n\r\n"},
		{"cl folded", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length: 5\r\n 5\r\n\r\n"},
		{"cl and chunked", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length: 5\r\nTransfer-Encoding: chunked\r\n\r\n" + smBody},
		{"cl and chunked with bad chunk", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length: 5\r\nTransfer-Encoding: chunked\r\n\r\nzz\r\nhello\r\n0\r\n\r\n"},
		{"te chunked", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding: chunked\r\n\r\n" + smBody},
		{"te chunked cased", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding: Chunked\r\n\r\n" + smBody},
		{"te chunked list", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding: gzip, chunked\r\n\r\n" + smBody},
		{"te chunked twice", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding: chunked\r\nTransfer-Encoding: chunked\r\n\r\n" + smBody},
		{"te folded", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding: chun\r\n ked\r\n\r\n" + smBody},
		{"te tab after colon", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding:\tchunked\r\n\r\n" + smBody},
		{"te space before colon", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding : chunked\r\n\r\n" + smBody},
		{"te unsupported", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding: gzip\r\n\r\n"},
		{"te empty", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding:\r\n\r\n"},
		{"chunk bare lf", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding: chunked\r\n\r\n5\nhello\r\n0\r\n\r\n"},
		{"chunk no crlf after data", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhelloX\r\n0\r\n\r\n"},
		{"chunk extension", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding: chunked\r\n\r\n5;x=y\r\nhello\r\n0\r\n\r\n"},
		{"chunk size too large", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding: chunked\r\n\r\nfffffffffffffffff\r\n"},
		{"chunk trailer with bad key", "POST / HTTP/1.1\r\nHost: h\r\nTransfer-Encoding: chunked\r\n\r\n0\r\nContent-Length: 5\r\n\r\n"},
		{"host missing", "POST / HTTP/1.1\r\nContent-Length: 0\r\n\r\n"},
		{"host empty", "POST / HTTP/1.1\r\nHost:\r\nContent-Length: 0\r\n\r\n"},
		{"host two equal", "POST / HTTP/1.1\r\nHost: h\r\nHost: h\r\nContent-Length: 0\r\n\r\n"},
		{"host two different", "POST / HTTP/1.1\r\nHost: h\r\nHost: other\r\nContent-Length: 0\r\n\r\n"},
		{"host with space", "POST / HTTP/1.1\r\nHost: a b\r\nContent-Length: 0\r\n\r\n"},
		{"host folded", "POST / HTTP/1.1\r\nHost: h\r\n x\r\nContent-Length: 0\r\n\r\n"},
		{"host absolute form", "POST http://h/ HTTP/1.1\r\nHost: h\r\nContent-Length: 0\r\n\r\n"},
		{"name with space", "POST / HTTP/1.1\r\nHost: h\r\nContent Length: 5\r\n\r\nhello"},
		{"name space before colon", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length : 5\r\n\r\nhello"},
		{"lf lines", "POST / HTTP/1.1\nHost: h\nContent-Length: 5\n\nhello"},
		{"cl zero with pipelined bytes", "POST / HTTP/1.1\r\nHost: h\r\nContent-Length: 0\r\n\r\nGET / HTTP/1.1\r\nHost: h\r\n\r\n"},
	}
}

// smGoFrames sends raw to the server with its write side closed, reads
// every response to EOF, and returns the status codes and the handler's
// framing lines in order.
func smGoFrames(t *testing.T, addr, raw string) (codes []int, frames []string) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, raw); err != nil {
		return nil, nil
	}
	if cw, ok := conn.(*net.TCPConn); ok {
		cw.CloseWrite()
	}
	br := bufio.NewReader(conn)
	for {
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			return codes, frames
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		codes = append(codes, resp.StatusCode)
		// the server's own error page is not a framing answer: only the
		// handler's lines count, so a rejected request contributes no frame
		if line, ok := strings.CutPrefix(string(body), "frame|"); ok {
			frames = append(frames, strings.TrimSpace(line))
		}
	}
}

// smXoFile is the generated test file: one test per case, comparing the
// framing both sides see.
func smXoFile(cases []smCase, frames []string) string {
	var b strings.Builder
	b.WriteString("// Generated by TestHttpSmuggle; not part of the tree.\n\n")
	b.WriteString(smFrameFn)
	for i, c := range cases {
		fmt.Fprintf(&b, "test \"smuggle case %d: %s\" {\n", i, c.name)
		fmt.Fprintf(&b, "  expect(sm_frame(%s)) == %s\n}\n\n", xoStr(c.raw), xoStr(frames[i]))
	}
	return b.String()
}

// smFrameFn is the Xo side of the comparison: the framing of every request
// parse_head finds in raw, read the way its own Head says, as
// kind|clen|bodyhex lines joined with ";".
const smFrameFn = `fn sm_frame(raw: Str) -> Str {
  let rb = raw.bytes()
  var off = 0
  var lines: List[Str] = []
  var guard = 0
  while guard < 4 && off < rb.len() {
    let p = parse_head(rb[off..rb.len()].to_str() ?? "", true)
    if p.res != OK {
      break
    }
    let h = p.head
    var used = p.used
    var kind = "cl"
    var n = h.clen
    var body: Bytes = b""
    if h.chunked {
      let c = chunked(rb[off + used..rb.len()].to_str() ?? "", true)
      if c.res != HB_OK {
        lines.push("err|${c.body.bytes().to_hex()}")
        break
      }
      kind = "chunked"
      body = c.body.bytes()
      used = used + c.used
    } else if n > 0 && off + used + n <= rb.len() {
      body = rb[off + used..off + used + n]
      used = used + n
    }
    lines.push("${kind}|${n}|${body.to_hex()}")
    off = off + used
    guard += 1
  }
  lines.join(";")
}
`

// TestHttpSmuggle compares both sides' framing of the smuggling table (see
// the file comment). It needs a loopback listener, so it runs outside
// -short; the backends are arm64 and LLVM (XO_SMUGGLE_BACKENDS overrides).
func TestHttpSmuggle(t *testing.T) {
	if testing.Short() {
		t.Skip("the smuggling table listens on loopback")
	}
	cases := smCases()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			// the body's own framing failed: both sides say so with the
			// bytes they managed to read, and answer 400
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "frame|err|%s", hex.EncodeToString(body))
			return
		}
		kind := "cl"
		if len(r.TransferEncoding) > 0 || r.ContentLength < 0 {
			kind = "chunked"
		}
		fmt.Fprintf(w, "frame|%s|%d|%s", kind, r.ContentLength, hex.EncodeToString(body))
	})}
	go srv.Serve(ln)
	defer srv.Close()
	addr := ln.Addr().String()
	frames := make([]string, len(cases))
	accepted, rejected := 0, 0
	for i, c := range cases {
		codes, fr := smGoFrames(t, addr, c.raw)
		frames[i] = strings.Join(fr, ";")
		if len(codes) > 0 && codes[0]/100 == 2 {
			accepted++
		} else {
			rejected++
		}
		t.Logf("%-28s go %v %q", c.name, codes, frames[i])
	}
	// the table must stay mixed and must compare real framing: all rejects,
	// or all empty frames, would make the comparison pass without comparing
	// anything
	nonEmpty := 0
	for _, f := range frames {
		if f != "" {
			nonEmpty++
		}
	}
	if accepted < 5 || rejected < 5 || nonEmpty < 5 {
		t.Fatalf("the table is lopsided: %d accepted, %d rejected, %d with framing", accepted, rejected, nonEmpty)
	}
	backends := envBackends("XO_SMUGGLE_BACKENDS", []string{"arm64", "llvm"})
	probeModule(t, "native/llrt/http1", "native/llrt/http1_test.xo", smXoFile(cases, frames), backends, len(cases))
}
