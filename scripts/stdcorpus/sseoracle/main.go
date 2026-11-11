// Command sseoracle prints the std/sse corpus (decision 0603 stage 1).
//
// The reference is the WHATWG event stream interpretation (HTML Living
// Standard 9.2.6), written out below as ref. Two independent checks keep
// it honest before anything is printed, and a disagreement stops the
// generator:
//
//   - the web-platform-tests eventsource format-* and event-data cases
//     (web-platform-tests/wpt eventsource/, BSD 3-Clause), their streams
//     as message.py, message2.py and last-event-id.py serve them and
//     their assertions transcribed;
//   - github.com/tmaxmax/go-sse v0.12.0's sse.Read on seeded random
//     streams. go-sse also dispatches an event that has only an id or an
//     event name, and its Data cannot tell no data line from one empty
//     one, so the two are compared on the events whose data is not empty.
//
// Each stream becomes an Xo test fed whole, a byte at a time and in
// seeded random chunks, with ref's events, last event ID and retry.
package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"

	sse "github.com/tmaxmax/go-sse"
)

type event struct{ typ, data, id string }

type result struct {
	events []event
	lastID string
	retry  int64 // -1: none
}

// ref is the WHATWG interpretation: lines end at CRLF, LF or CR; a BOM at
// the start is dropped; an unterminated last line and a pending event at
// the end are dropped.
func ref(b []byte) result {
	b = bytes.TrimPrefix(b, []byte("\xEF\xBB\xBF"))
	r := result{retry: -1}
	var typ string
	var data strings.Builder
	haveData := false
	for {
		i := bytes.IndexAny(b, "\r\n")
		if i < 0 {
			break
		}
		line := b[:i]
		if b[i] == '\r' && i+1 < len(b) && b[i+1] == '\n' {
			b = b[i+2:]
		} else {
			b = b[i+1:]
		}
		if len(line) == 0 {
			if haveData {
				d := data.String()
				t := typ
				if t == "" {
					t = "message"
				}
				r.events = append(r.events, event{t, d[:len(d)-1], r.lastID})
			}
			typ, haveData = "", false
			data.Reset()
			continue
		}
		if line[0] == ':' {
			continue
		}
		name, value := line, []byte{}
		if k := bytes.IndexByte(line, ':'); k >= 0 {
			name, value = line[:k], line[k+1:]
			value = bytes.TrimPrefix(value, []byte(" "))
		}
		v := whatwgUTF8(value)
		switch string(name) {
		case "data":
			data.WriteString(v)
			data.WriteByte('\n')
			haveData = true
		case "event":
			typ = v
		case "id":
			if !strings.ContainsRune(v, 0) {
				r.lastID = v
			}
		case "retry":
			ok := len(value) > 0
			for _, c := range value {
				if c < '0' || c > '9' {
					ok = false
				}
			}
			if ok {
				if n, err := strconv.ParseInt(string(value), 10, 64); err == nil {
					r.retry = n
				}
			}
		}
	}
	return r
}

type wptCase struct {
	name string
	body string
	// the data of the first events, in order, as the test asserts them
	want []string
	// the lastEventId of event wantIDAt, when the test asserts it
	wantID   string
	wantIDAt int
}

// message.py's body: message, then "\n\n" unless newline=none, then "\n".
func msg(m string, newline bool) string {
	if newline {
		return m + "\n\n\n"
	}
	return m + "\n"
}

var wpt = []wptCase{
	{name: "format-bom", body: msg("\uFEFFdata:1\n\n\uFEFFdata:2\n\ndata:3", true), want: []string{"1", "3"}},
	{name: "format-bom-2", body: msg("\uFEFF\uFEFFdata:1\n\ndata:2\n\ndata:3", true), want: []string{"2", "3"}},
	{name: "format-bom-split", body: "\xEF\xBB\xBFdata:1\n\ndata:2\n\n", want: []string{"1", "2"}},
	{name: "format-comments", body: msg("data:1\r:\x00\n:\r\ndata:2\n:"+strings.Repeat("x", 2048)+"\rdata:3\n:data:fail\r:"+strings.Repeat("x", 2048)+"\ndata:4\n", false), want: []string{"1\n2\n3\n4"}},
	{name: "format-data-before-final-empty-line", body: msg("retry:1000\ndata:test1\n\nid:test\ndata:test2", false), want: []string{"test1"}, wantID: "", wantIDAt: 0},
	{name: "format-field-data", body: msg("data:\n\ndata\ndata\n\ndata:test", true), want: []string{"", "\n", "test"}},
	{name: "format-field-event-empty", body: msg("event: \ndata:data", true), want: []string{"data"}},
	{name: "format-field-event", body: msg("event:test\ndata:x\n\ndata:x", true), want: []string{"x", "x"}},
	{name: "format-field-id (last-event-id.py)", body: "id: \u2026\nretry: 200\ndata: hello\n\n", want: []string{"hello"}, wantID: "\u2026", wantIDAt: 0},
	{name: "format-field-parsing", body: msg("data:\x00\ndata:  2\rData:1\ndata\x00:2\ndata:1\r\x00data:4\nda-ta:3\rdata_5\ndata:3\rdata:\r\n data:32\ndata:4\n", false), want: []string{"\x00\n 2\n1\n3\n\n4"}},
	{name: "format-field-retry-empty", body: msg("retry\ndata:test", true), want: []string{"test"}},
	{name: "format-field-retry", body: msg("retry:03000\ndata:x", true), want: []string{"x"}},
	{name: "format-field-retry-bogus", body: msg("retry:3000\nretry:1000x\ndata:x", true), want: []string{"x"}},
	{name: "format-field-retry-bogus-2", body: msg("retry:1000\nretry:5000x\ndata:x", true), want: []string{"x"}},
	{name: "format-field-unknown", body: msg("data:test\n data\ndata\nfoobar:xxx\njustsometext\n:thisisacommentyay\ndata:test", true), want: []string{"test\n\ntest"}},
	{name: "format-leading-space", body: msg("data:\ttest\rdata: \ndata:test", true), want: []string{"\ttest\n\ntest"}},
	{name: "format-newlines", body: msg("data:test\r\ndata\ndata:test\r\n\r", false), want: []string{"test\n\ntest"}},
	{name: "format-null-character", body: msg("data:\x00\n\n", true), want: []string{"\x00"}},
	{name: "format-utf-8", body: msg("data:ok\u2026", true), want: []string{"ok\u2026"}},
	{name: "event-data (message2.py)", body: "data:msg\ndata: msg\n\n:\nfalsefield:msg\n\nfalsefield:msg\nData:data\n\ndata\n\ndata:end\n\n", want: []string{"msg\nmsg", "", "end"}},
}

// the retry values the WPT retry tests time
var wptRetry = map[string]int64{
	"format-field-retry":         3000,
	"format-field-retry-bogus":   3000,
	"format-field-retry-bogus-2": 1000,
}

// the Encoding Standard's examples of maximal subparts
var utf8Cases = map[string]string{
	"\xff\xfe":          "\uFFFD\uFFFD",
	"\xe2\x80a":         "\uFFFDa",
	"\xf0\x9f\x98":      "\uFFFD",
	"\xed\xa0\x80":      "\uFFFD\uFFFD\uFFFD",
	"\xc0\xaf":          "\uFFFD\uFFFD",
	"\xf0\x9f\x98\x80":  "\U0001F600",
	"a\xf4\x90\x80\x80": "a\uFFFD\uFFFD\uFFFD\uFFFD",
}

func checkWPT() {
	for in, want := range utf8Cases {
		if got := whatwgUTF8([]byte(in)); got != want {
			panic(fmt.Sprintf("utf-8 %q: %q, want %q", in, got, want))
		}
	}
	for _, c := range wpt {
		r := ref([]byte(c.body))
		if len(r.events) < len(c.want) {
			panic(fmt.Sprintf("wpt %s: %d events, want at least %d", c.name, len(r.events), len(c.want)))
		}
		for i, w := range c.want {
			if r.events[i].data != w {
				panic(fmt.Sprintf("wpt %s: event %d data %q, want %q", c.name, i, r.events[i].data, w))
			}
		}
		if c.wantID != "" || c.name == "format-data-before-final-empty-line" {
			if got := r.events[c.wantIDAt].id; got != c.wantID {
				panic(fmt.Sprintf("wpt %s: lastEventId %q, want %q", c.name, got, c.wantID))
			}
		}
		if w, ok := wptRetry[c.name]; ok && r.retry != w {
			panic(fmt.Sprintf("wpt %s: retry %d, want %d", c.name, r.retry, w))
		}
	}
}

// whatwgUTF8 is the Encoding Standard's UTF-8 decoder (8.1.1): each
// maximal subpart of an invalid sequence is one U+FFFD.
func whatwgUTF8(b []byte) string {
	var o strings.Builder
	need, seen, lo, hi := 0, 0, 0x80, 0xBF
	var cp rune
	for i := 0; i < len(b); i++ {
		c := int(b[i])
		if need == 0 {
			switch {
			case c < 0x80:
				o.WriteByte(b[i])
			case c >= 0xC2 && c <= 0xDF:
				need, cp = 1, rune(c&0x1F)
			case c >= 0xE0 && c <= 0xEF:
				if c == 0xE0 {
					lo = 0xA0
				}
				if c == 0xED {
					hi = 0x9F
				}
				need, cp = 2, rune(c&0xF)
			case c >= 0xF0 && c <= 0xF4:
				if c == 0xF0 {
					lo = 0x90
				}
				if c == 0xF4 {
					hi = 0x8F
				}
				need, cp = 3, rune(c&7)
			default:
				o.WriteRune(0xFFFD)
			}
			continue
		}
		if c < lo || c > hi {
			need, seen, lo, hi, cp = 0, 0, 0x80, 0xBF, 0
			o.WriteRune(0xFFFD)
			i-- // the byte starts afresh
			continue
		}
		lo, hi = 0x80, 0xBF
		cp = cp<<6 | rune(c&0x3F)
		seen++
		if seen == need {
			o.WriteRune(cp)
			need, seen, cp = 0, 0, 0
		}
	}
	if need != 0 {
		o.WriteRune(0xFFFD)
	}
	return o.String()
}

// goSSE is go-sse's events with data, as ref's events with data.
func goSSE(b []byte) []event {
	var out []event
	for e, err := range sse.Read(bytes.NewReader(b), &sse.ReadConfig{MaxEventSize: 1 << 20}) {
		if err != nil {
			panic("go-sse: " + err.Error())
		}
		if e.Data == "" {
			continue
		}
		t := e.Type
		if t == "" {
			t = "message"
		}
		// go-sse keeps the bytes as they came; decode them as ref does
		out = append(out, event{whatwgUTF8([]byte(t)), whatwgUTF8([]byte(e.Data)), whatwgUTF8([]byte(e.LastEventID))})
	}
	return out
}

func withData(es []event) []event {
	var out []event
	for _, e := range es {
		if e.data != "" {
			out = append(out, e)
		}
	}
	return out
}

func sameEvents(a, b []event) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var (
	names  = []string{"data", "data", "data", "event", "id", "retry", "Data", "dat", "x", ""}
	values = []string{"", " ", "a", " b", "  c", "msg", "1", "042", "7x", "-1", "\u00e9t\u00e9", "\x00", "a:b", ":", "\xff\xfe"}
	ends   = []string{"\n", "\r\n", "\r"}
)

// randStream is a stream of random lines: fields with and without a
// colon, comments, empty lines, every line ending, maybe a BOM and maybe
// an unterminated last line.
func randStream(rg *rand.Rand) []byte {
	var b bytes.Buffer
	if rg.Intn(5) == 0 {
		b.WriteString("\uFEFF")
	}
	n := 3 + rg.Intn(14)
	for i := 0; i < n; i++ {
		switch k := rg.Intn(10); {
		case k < 3:
		case k == 3:
			b.WriteString(":" + values[rg.Intn(len(values))])
		case k == 4:
			b.WriteString(names[rg.Intn(len(names))])
		default:
			b.WriteString(names[rg.Intn(len(names))] + ":" + values[rg.Intn(len(values))])
		}
		if i < n-1 || rg.Intn(3) > 0 {
			b.WriteString(ends[rg.Intn(len(ends))])
		}
	}
	return b.Bytes()
}

func xq(s string) string {
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

func xbytes(b []byte) string {
	var s strings.Builder
	s.WriteString(`b"`)
	for _, c := range b {
		if c >= 0x20 && c < 0x7f && c != '"' && c != '\\' && c != '$' {
			s.WriteByte(c)
		} else {
			fmt.Fprintf(&s, `\x%02x`, c)
		}
	}
	s.WriteByte('"')
	return s.String()
}

// render is the text the Xo test's show makes of a result.
func render(r result) string {
	var b strings.Builder
	for _, e := range r.events {
		b.WriteString(e.typ + "\x01" + e.id + "\x01" + e.data + "\x02")
	}
	retry := "none"
	if r.retry >= 0 {
		retry = strconv.FormatInt(r.retry, 10)
	}
	b.WriteString("last=" + r.lastID + " retry=" + retry)
	return b.String()
}

// splits is b in seeded random chunks of 1 to 7 bytes.
func splits(rg *rand.Rand, b []byte) [][]byte {
	var out [][]byte
	for len(b) > 0 {
		n := 1 + rg.Intn(7)
		if n > len(b) {
			n = len(b)
		}
		out = append(out, b[:n])
		b = b[n:]
	}
	return out
}

func xchunks(cs [][]byte) string {
	var parts []string
	for _, c := range cs {
		parts = append(parts, xbytes(c))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func emit(o *strings.Builder, rg *rand.Rand, name string, body []byte) {
	want := xq(render(ref(body)))
	fmt.Fprintf(o, "\ntest %s {\n", xq(name))
	fmt.Fprintf(o, "  let body = %s\n", xbytes(body))
	fmt.Fprintf(o, "  expect(show([body])) == %s\n", want)
	fmt.Fprintf(o, "  expect(show(bytewise(body))) == %s\n", want)
	fmt.Fprintf(o, "  expect(show(%s)) == %s\n", xchunks(splits(rg, body)), want)
	fmt.Fprintf(o, "}\n")
}

func main() {
	checkWPT()
	rg := rand.New(rand.NewSource(603))
	var o strings.Builder
	o.WriteString(`// Generated by scripts/stdcorpus/sseoracle: the WHATWG event stream
// interpretation, checked against the web-platform-tests eventsource
// cases (BSD 3-Clause) and github.com/tmaxmax/go-sse v0.12.0 (MIT).
// Do not edit: regenerate with go run ./scripts/stdcorpus sse.

use std/http
use std/sse

/// The events of chunks fed in order, then the stream's state, in the
/// oracle's render: type, id and data split by \u{1}, events by \u{2}.
fn show(chunks: List[Bytes]) -> Str {
  var p = sse.parser()
  var o = StrBuilder.new()
  for c in chunks {
    match p.feed(c) {
      Ok(evs) => {
        for e in evs {
          o.write("${e.event}\u{1}${e.id}\u{1}${e.data}\u{2}")
        }
      }
      Err(e) => return "error: ${e.display()}"
    }
  }
  let retry = match p.retry() {
    Some(n) => "${n}"
    None => "none"
  }
  o.write("last=${p.last_event_id()} retry=${retry}")
  o.freeze()
}

fn bytewise(b: Bytes) -> List[Bytes] {
  var out: List[Bytes] = []
  for i in 0..b.len() {
    out.push(b[i..i + 1])
  }
  out
}
`)
	for _, c := range wpt {
		emit(&o, rg, "wpt "+c.name, []byte(c.body))
	}
	for i := 0; i < 60; i++ {
		body := randStream(rg)
		r := ref(body)
		if g, w := goSSE(body), withData(r.events); !sameEvents(g, w) {
			fmt.Fprintf(os.Stderr, "sseoracle: go-sse and ref differ on %q:\n go-sse %q\n ref    %q\n", body, g, w)
			os.Exit(1)
		}
		emit(&o, rg, fmt.Sprintf("random stream %d (go-sse agrees on its events with data)", i), body)
	}
	o.WriteString(fixed)
	fmt.Print(o.String())
}

// fixed is the Reader and writer tests: the format's rules, not an
// oracle's output.
const fixed = `
/// A byte source that hands out chunks in turn, then fails with fail or
/// ends.
struct Chunks {
  cs: List[Bytes]
  at: Mutex[Int]
  fail: Str?
}

fn chunks(cs: List[Bytes], fail: Str? = None) -> Chunks { Chunks{cs: cs, at: Mutex[Int].new(0), fail: fail} }

fn (c: Chunks) read(max: Int) -> Bytes? ! Str {
  let i = c.at.with(fn(var n: Int) { n += 1; n - 1 })
  if let Some(b) = c.cs.get(i) {
    return Ok(Some(b))
  }
  match c.fail {
    Some(m) => Err(m)
    None => Ok(None)
  }
}

// max_line defaults to a literal, not sse.MAX_LINE: with the constant, the
// arm64 backend fails "all reads virtual register N before it is written"
// (2026-11-11, xo main 68b9c7033; scratchpad repro handed to the lead)
fn all(c: Chunks, max_line: Int = 1048576, last: Str = "") -> Str {
  var r = sse.reader(fn(n: Int) -> Bytes? ! Str { c.read(n) }, max_line: max_line, last_event_id: last)
  var o = StrBuilder.new()
  while true {
    match r.next() {
      Ok(Some(e)) => o.write("${e.event}/${e.id}/${e.data};")
      Ok(None) => break
      Err(sse.ReadErr.Source(m)) => {
        o.write("source error ${m};")
        break
      }
      Err(sse.ReadErr.Format(e)) => {
        o.write("format error ${e.display()};")
        break
      }
    }
  }
  let after = match r.next() {
    Ok(None) => "then none"
    _ => "then more"
  }
  o.write(after)
  o.write(" last=${r.last_event_id()} retry=${r.retry() ?? -1}")
  o.freeze()
}

test "a Reader gives the events of its source's chunks, an unfinished last event dropped" {
  let c = chunks([b"retry: 250\nid: 1\ndata: a", b"\r\n\r", b"\nevent: e\ndata: b\n\n", b"data: unfinished\n"])
  expect(all(c)) == "message/1/a;e/1/b;then none last=1 retry=250"
}

test "a Reader returns its source's error after the events before it, then None" {
  let c = chunks([b"data: a\n\ndata: b"], fail: Some("reset"))
  expect(all(c)) == "message//a;source error reset;then none last= retry=-1"
}

test "a Reader stops at a line over its limit" {
  let c = chunks([b"data: ok\n\n", b"data: 0123456789\n\n"])
  expect(all(c, max_line: 8)) == "message//ok;format error sse: line longer than 8 bytes;then none last= retry=-1"
}

test "an unterminated line over the limit is an error before its end" {
  let c = chunks([b"data: 01234", b"56789"])
  expect(all(c, max_line: 8)) == "format error sse: line longer than 8 bytes;then none last= retry=-1"
}

test "a Reader starts from the cursor it is given" {
  expect(all(chunks([b"data: a\n\nid\ndata: b\n\n"]), last: "41")) == "message/41/a;message//b;then none last= retry=-1"
}

test "a Parser's error sticks" {
  var p = sse.parser(max_line: 4)
  expect(p.feed(b"data: long\n").is_err()) == true
  expect(p.feed(b"\n").is_err()) == true
}

test "encode writes each field and data line" {
  expect(sse.encode("a\nb", event: "e", id: Some("9"), retry: Some(5))) == "event: e\nid: 9\nretry: 5\ndata: a\ndata: b\n\n"
  expect(sse.encode("")) == "data\n\n"
  expect(sse.encode("x", event: "message")) == "data: x\n\n"
  expect(sse.encode("x", id: Some(""))) == "id\ndata: x\n\n"
  expect(sse.encode(" lead\r\nend\r")) == "data:  lead\ndata: end\ndata\n\n"
}

test "an encoded event reads back, its line breaks as LF" {
  for d in ["", "x", "a\nb", "a\r\nb\rc", "\n", " spaced ", ": colon", "\u{e9}t\u{e9}"] {
    var p = sse.parser()
    let evs = p.feed(sse.encode(d, event: "kind", id: Some("7")).bytes()).ok() ?? []
    expect(evs.len()) == 1
    expect(evs[0].event) == "kind"
    expect(evs[0].id) == "7"
    expect(evs[0].data) == d.replace("\r\n", "\n").replace("\r", "\n")
  }
}

test "comment writes one comment line for each line" {
  expect(sse.comment()) == ":\n"
  expect(sse.comment("a\nb")) == ": a\n: b\n"
  var p = sse.parser()
  expect((p.feed((sse.comment("x\ny") + sse.encode("z")).bytes()).ok() ?? []).len()) == 1
}

test "the event stream headers" {
  expect(sse.headers().get("Content-Type")) == Some("text/event-stream")
  expect(sse.headers().get("Cache-Control")) == Some("no-cache")
}

test "subscribe reconnects with Last-Event-ID and the server's retry" {
  let calls = Mutex[Int].new(0)
  os.net.handle("GET https://ev.test/s", fn(req: http.Request) -> http.Response {
    let n = calls.with(fn(var c: Int) -> Int {
      c += 1
      c
    })
    let body = if n == 1 {
      "retry: 50\nid: 1\ndata: a\n\nid: 2\ndata: b\n\ndata: cut"
    } else if n == 2 {
      "data: c\n\n"
    } else {
      ""
    }
    if n >= 3 {
      return http.text(204, "")
    }
    http.text(200, body).with_header("Content-Type", "text/event-stream")
  })
  var s = sse.subscribe(http.client(os.net), os.clock, "https://ev.test/s")
  var got: List[Str] = []
  while true {
    match s.next() {
      Ok(Some(ev)) => got.push("${ev.data}@${ev.id}")
      Ok(None) => break
      Err(e) => {
        got.push("err ${e.display()}")
        break
      }
    }
  }
  expect(got) == ["a@1", "b@2", "c@2"]
  var lids: List[Str] = []
  for r in os.net.requests() {
    lids.push(r.header("Last-Event-ID") ?? "-")
  }
  expect(lids) == ["-", "2", "2"]
  expect(os.clock.sleeps()) == [50ms, 50ms]
  // the server's retry replaced the default reconnection time
  expect(s.retry_ms()) == 50
}

test "a server sends events with send as they happen, and a Source keeps the last id" {
  os.net.handle("GET https://ev.test/live", fn(_r: http.Request) -> http.Response {
    http.stream(200, fn(w: http.BodyWriter) -> () ! IoErr {
      sse.send(w, sse.comment("hello"))?
      sse.send(w, sse.encode("one", id: Some("7")))?
      sse.send(w, sse.encode("two", event: "tick"))
    }).with_header("Content-Type", "text/event-stream")
  })
  var s = sse.subscribe(http.client(os.net), os.clock, "https://ev.test/live", max_retries: 0)
  expect(s.last_event_id()) == ""
  var got: List[Str] = []
  while got.len() < 2 {
    match s.next() {
      Ok(Some(ev)) => got.push("${ev.event}:${ev.data}@${ev.id}")
      _ => break
    }
  }
  // the comment is no event; the second event has no id field, so the
  // last one carries over
  expect(got) == ["message:one@7", "tick:two@7"]
  expect(s.last_event_id()) == "7"
}

test "a Parser started from a saved cursor reports it until an id field" {
  var p = sse.parser()
  p.set_last_event_id("41")
  expect(p.last_event_id()) == "41"
  let evs = p.feed("data: a\n\nid: 42\ndata: b\n\n".bytes()).ok() ?? []
  var ids: List[Str] = []
  for e in evs {
    ids.push(e.id)
  }
  expect(ids) == ["41", "42"]
  expect(p.last_event_id()) == "42"
}

test "subscribe: a refused stream fails, connect failures give up after max_retries" {
  os.net.handle("GET https://ev.test/html", fn(_r: http.Request) -> http.Response { http.text(200, "<p>") })
  var s = sse.subscribe(http.client(os.net), os.clock, "https://ev.test/html")
  match s.next() {
    Err(sse.SourceErr.Failed(_)) => {}
    _ => expect(false) == true
  }
  os.net.fail("GET https://ev.test/down", http.ClientErr.Timeout)
  var d = sse.subscribe(http.client(os.net), os.clock, "https://ev.test/down", retry_ms: 10, max_retries: 2)
  expect(d.retry_ms()) == 10
  match d.next() {
    Err(sse.SourceErr.GaveUp(http.ClientErr.Timeout)) => {}
    _ => expect(false) == true
  }
  expect(os.clock.sleeps()) == [10ms, 10ms]
}
`
