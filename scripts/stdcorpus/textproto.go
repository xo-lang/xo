package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/textproto"
	"sort"
	"strings"
)

// textprotoErr is a textproto error as the Xo corpus spells it: `code N
// MSG` for an *Error, `eof` for io.EOF, `ueof` for io.ErrUnexpectedEOF,
// else `proto TEXT`.
func textprotoErr(err error) string {
	var te *textproto.Error
	switch {
	case errors.As(err, &te):
		return fmt.Sprintf("code %d %s", te.Code, te.Msg)
	case err == io.EOF:
		return "eof"
	case err == io.ErrUnexpectedEOF:
		return "ueof"
	}
	return "proto " + err.Error()
}

func textprotoReader(s string) (*textproto.Reader, *bufio.Reader) {
	br := bufio.NewReaderSize(strings.NewReader(s), 1<<20)
	return textproto.NewReader(br), br
}

// textprotoHeader is ReadMIMEHeader's result: `ok K=V,V|K=V REST` with the
// keys sorted (MIMEHeader is a map), `trunc` instead of `ok` when it ended
// at io.EOF after some fields, else the error.
func textprotoHeader(s string) string {
	r, br := textprotoReader(s)
	m, err := r.ReadMIMEHeader()
	tag := "ok"
	if err == io.EOF && len(m) > 0 {
		tag = "trunc"
	} else if err != nil {
		return "err " + textprotoErr(err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+strings.Join(m[k], ","))
	}
	rest, _ := io.ReadAll(br)
	return tag + " " + strings.Join(parts, "|") + " " + string(rest)
}

// textprotoResponse is ReadResponse's result: `ok CODE MSG` or the error.
func textprotoResponse(s string, expect int) string {
	r, _ := textprotoReader(s)
	code, msg, err := r.ReadResponse(expect)
	if err != nil {
		return "err " + textprotoErr(err)
	}
	return fmt.Sprintf("ok %d %s", code, msg)
}

// textprotoCodeLine is ReadCodeLine's result: `ok CODE MSG` or the error.
func textprotoCodeLine(s string, expect int) string {
	r, _ := textprotoReader(s)
	code, msg, err := r.ReadCodeLine(expect)
	if err != nil {
		return "err " + textprotoErr(err)
	}
	return fmt.Sprintf("ok %d %s", code, msg)
}

// textprotoContinued is ReadContinuedLine called until an error: each
// line, `|` joined, then the error.
func textprotoContinued(s string) string {
	r, _ := textprotoReader(s)
	var out []string
	for {
		l, err := r.ReadContinuedLine()
		if err != nil {
			return strings.Join(out, "|") + " err " + textprotoErr(err)
		}
		out = append(out, l)
	}
}

// textprotoDotLines is ReadDotLines's result: the lines `|` joined then
// `ok`, or the error.
func textprotoDotLines(s string) string {
	r, _ := textprotoReader(s)
	ls, err := r.ReadDotLines()
	if err != nil {
		return "err " + textprotoErr(err)
	}
	return strings.Join(ls, "|") + " ok"
}

// textprotoDel is the header's sorted fields after MIMEHeader.Del of key,
// or `err` when the block does not parse.
func textprotoDel(s, key string) string {
	r, _ := textprotoReader(s)
	m, err := r.ReadMIMEHeader()
	if err != nil && !(err == io.EOF && len(m) > 0) {
		return "err"
	}
	m.Del(key)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+strings.Join(m[k], ","))
	}
	return strings.Join(parts, "|")
}

// textprotoDot is ReadDotBytes's result and the dot encoding of s.
func textprotoDot(s string) (string, string) {
	r, br := textprotoReader(s)
	b, err := r.ReadDotBytes()
	got := "ok " + string(b)
	if err != nil {
		got = "err " + textprotoErr(err)
	} else {
		rest, _ := io.ReadAll(br)
		got += " REST " + string(rest)
	}
	var w strings.Builder
	bw := bufio.NewWriter(&w)
	d := textproto.NewWriter(bw).DotWriter()
	d.Write([]byte(s))
	d.Close()
	return got, w.String()
}

func textprotoPick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

func textprotoCorpus() string {
	r := rand.New(rand.NewSource(598))
	var o strings.Builder
	o.WriteString(header("net/textproto's ReadMIMEHeader, ReadResponse, ReadDotBytes, DotWriter and CanonicalMIMEHeaderKey on seeded random header blocks, reply lines and dot-encoded text"))
	o.WriteString("use std/textproto\n\n/// (key, Go's CanonicalMIMEHeaderKey)\nconst KEYS: List[(Str, Str)] = [\n")
	keyParts := []string{"a", "B", "content", "TYPE", "-", "x", "1", "_", " ", "é", ":", "\t", "~", "", "ü", "Accept"}
	for i := 0; i < 400; i++ {
		var b strings.Builder
		for j, n := 0, 1+r.Intn(4); j < n; j++ {
			b.WriteString(textprotoPick(r, keyParts))
		}
		k := b.String()
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(k), xq(textproto.CanonicalMIMEHeaderKey(k)))
	}
	o.WriteString("]\n\n/// (header block, Go's ReadMIMEHeader: ok|trunc K=V,V|... REST or err ERROR, Go's ReadContinuedLine to the end, key, the fields after Del(key))\nconst HEADERS: List[(Str, Str, Str, Str, Str)] = [\n")
	names := []string{"content-type", "X-A", "x-a", "Subject", "SID ", "a b", "", "Foo\tBar", "k_1", "from", "é"}
	values := []string{"text/plain", " v ", "", "a: b", "x\ty", "é", "\x01", "a\x7fb", "\t lead", "trail \t", "日本"}
	ends := []string{"\r\n", "\n"}
	for i := 0; i < 1500; i++ {
		var b strings.Builder
		if r.Intn(20) == 0 {
			b.WriteString(textprotoPick(r, []string{" ", "\t"}))
		}
		for j, n := 0, r.Intn(5); j < n; j++ {
			switch r.Intn(12) {
			case 0:
				b.WriteString("no colon")
			case 1:
				b.WriteString(textprotoPick(r, []string{" ", "\t", "  \t"}) + textprotoPick(r, values))
			default:
				b.WriteString(textprotoPick(r, names) + ":" + textprotoPick(r, values))
			}
			b.WriteString(textprotoPick(r, ends))
		}
		switch r.Intn(6) {
		case 0:
			// the input ends without a blank line
		case 1:
			b.WriteString(textprotoPick(r, ends) + "body")
		default:
			b.WriteString(textprotoPick(r, ends))
		}
		s := b.String()
		dk := textprotoPick(r, names)
		fmt.Fprintf(&o, "  (%s, %s, %s, %s, %s),\n", xq(s), xq(textprotoHeader(s)), xq(textprotoContinued(s)), xq(dk), xq(textprotoDel(s, dk)))
	}
	o.WriteString("]\n\n/// (reply, expected code, Go's ReadResponse: ok CODE MSG or err ERROR, Go's ReadCodeLine the same way)\nconst RESPONSES: List[(Str, Int, Str, Str)] = [\n")
	codes := []string{"250", "550", "220", "abc", "099", "25", "2500"}
	for i := 0; i < 1200; i++ {
		var b strings.Builder
		code := textprotoPick(r, codes)
		n := 1 + r.Intn(4)
		for j := 0; j < n; j++ {
			c := code
			if r.Intn(8) == 0 {
				c = textprotoPick(r, codes)
			}
			sep := "-"
			if j == n-1 {
				sep = " "
			}
			if r.Intn(10) == 0 {
				b.WriteString("free text")
			} else {
				b.WriteString(c + sep + textprotoPick(r, []string{"ok", "", "a b", "5.1.1 no", " lead"}))
			}
			b.WriteString(textprotoPick(r, ends))
		}
		if r.Intn(10) == 0 {
			s := b.String()
			b.Reset()
			b.WriteString(strings.TrimRight(s, "\r\n"))
		}
		expect := []int{0, 2, 25, 250, 550, 5}[r.Intn(6)]
		s := b.String()
		fmt.Fprintf(&o, "  (%s, %d, %s, %s),\n", xq(s), expect, xq(textprotoResponse(s, expect)), xq(textprotoCodeLine(s, expect)))
	}
	o.WriteString("]\n\n/// (text, Go's ReadDotBytes: ok TEXT REST REST or err ERROR, Go's DotWriter output, Go's ReadDotLines: LINE|LINE ok or err ERROR)\nconst DOTS: List[(Str, Str, Str, Str)] = [\n")
	dotParts := []string{"a", ".", "..", "\r", "\n", "\r\n", ".\r\n", ".\n", "x.y", " "}
	for i := 0; i < 1000; i++ {
		var b strings.Builder
		for j, n := 0, r.Intn(8); j < n; j++ {
			b.WriteString(textprotoPick(r, dotParts))
		}
		s := b.String()
		got, enc := textprotoDot(s)
		fmt.Fprintf(&o, "  (%s, %s, %s, %s),\n", xq(s), xq(got), xq(enc), xq(textprotoDotLines(s)))
	}
	o.WriteString("]\n")
	o.WriteString(textprotoTests)
	return o.String()
}

const textprotoTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

fn err_text(e: textproto.TextErr) -> Str {
  match e {
    textproto.TextErr.Code(c, m) => "code ${c} ${m}"
    textproto.TextErr.Eof => "eof"
    textproto.TextErr.UnexpectedEof => "ueof"
    textproto.TextErr.Protocol(m) => "proto ${m}"
  }
}

fn header_of(s: Str) -> Str {
  var r = textproto.reader(s.bytes())
  match r.read_header() {
    Ok(h) => {
      let fields = h.keys().sort().map(fn(k) { "${k}=${h.values(k).join(",")}" }).join("|")
      "${if r.header_truncated() { "trunc" } else { "ok" }} ${fields} ${r.rest().to_str_lossy()}"
    }
    Err(e) => "err ${err_text(e)}"
  }
}

fn response_of(s: Str, expect: Int) -> Str {
  var r = textproto.reader(s.bytes())
  match r.read_response(expect) {
    Ok((c, m)) => "ok ${c} ${m}"
    Err(e) => "err ${err_text(e)}"
  }
}

fn code_line_of(s: Str, expect: Int) -> Str {
  var r = textproto.reader(s.bytes())
  match r.read_code_line(expect) {
    Ok((c, m)) => "ok ${c} ${m}"
    Err(e) => "err ${err_text(e)}"
  }
}

fn continued_of(s: Str) -> Str {
  var r = textproto.reader(s.bytes())
  var out: List[Str] = []
  while true {
    match r.read_continued_line() {
      Ok(l) => out.push(l)
      Err(e) => return "${out.join("|")} err ${err_text(e)}"
    }
  }
  ""
}

fn del_of(s: Str, key: Str) -> Str {
  var r = textproto.reader(s.bytes())
  match r.read_header() {
    Ok(h0) => {
      let h = h0.del(key)
      h.keys().sort().map(fn(k) { "${k}=${h.values(k).join(",")}" }).join("|")
    }
    Err(_) => "err"
  }
}

fn dot_lines_of(s: Str) -> Str {
  var r = textproto.reader(s.bytes())
  match r.read_dot_lines() {
    Ok(ls) => "${ls.join("|")} ok"
    Err(e) => "err ${err_text(e)}"
  }
}

fn dot_of(s: Str) -> Str {
  var r = textproto.reader(s.bytes())
  match r.read_dot_bytes() {
    Ok(b) => "ok ${b.to_str_lossy()} REST ${r.rest().to_str_lossy()}"
    Err(e) => "err ${err_text(e)}"
  }
}

test "canonical keys are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (k, want) in KEYS {
    let got = textproto.canonical_key(k)
    if got != want {
      bad.push((k, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "header blocks are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (s, want, cont, dk, dwant) in HEADERS {
    let got = header_of(s)
    if got != want {
      bad.push((s, got, want))
    }
    let gc = continued_of(s)
    if gc != cont {
      bad.push((s, gc, cont))
    }
    let gd = del_of(s, dk)
    if gd != dwant {
      bad.push((s, gd, dwant))
    }
  }
  expect(first_bad(bad)) == []
}

test "replies are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (s, expect_code, want, cwant) in RESPONSES {
    let got = response_of(s, expect_code)
    if got != want {
      bad.push((s, got, want))
    }
    let gc = code_line_of(s, expect_code)
    if gc != cwant {
      bad.push((s, gc, cwant))
    }
  }
  expect(first_bad(bad)) == []
}

test "dot reading and writing are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (s, want, enc, lwant) in DOTS {
    let gl = dot_lines_of(s)
    if gl != lwant {
      bad.push((s, gl, lwant))
    }
    let got = dot_of(s)
    if got != want {
      bad.push((s, got, want))
    }
    let genc = textproto.dot_encode(s.bytes()).to_str_lossy()
    if genc != enc {
      bad.push((s, genc, enc))
    }
  }
  expect(first_bad(bad)) == []
}
`
