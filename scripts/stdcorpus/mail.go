package main

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"math/rand"
	"net/mail"
	"net/textproto"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// goTestStrings are the string literals of a Go test file that are valid
// UTF-8 (an Xo Str cannot hold the others), deduplicated, in order.
func goTestStrings(rel string) []string {
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(runtime.GOROOT(), "src", rel), nil, 0)
	if err != nil {
		panic(err)
	}
	seen := map[string]bool{}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		if bl, ok := n.(*ast.BasicLit); ok && bl.Kind == token.STRING {
			if s, err := strconv.Unquote(bl.Value); err == nil && utf8.ValidString(s) && !seen[s] && len(s) < 300 {
				seen[s] = true
				out = append(out, s)
			}
		}
		return true
	})
	return out
}

// mailAddress is std/rt's parse_address result as the test prints it.
func mailAddress(s string) string {
	a, err := mail.ParseAddress(s)
	if err != nil {
		return "err " + err.Error()
	}
	name, addr := textOf(a.Name), textOf(a.Address)
	return "ok " + name + "|" + addr + "|" + (&mail.Address{Name: name, Address: addr}).String()
}

func mailList(s string) string {
	as, err := mail.ParseAddressList(s)
	if err != nil {
		return "err " + err.Error()
	}
	var parts []string
	for _, a := range as {
		parts = append(parts, textOf(a.Name)+"|"+textOf(a.Address))
	}
	return "ok " + strings.Join(parts, ";")
}

// mailDate is std/rt's parse_date (mail.ParseDate in UTC, then Time's
// range) as unix seconds.
func mailDate(s string) string {
	t, err := mail.ParseDate(s)
	if err != nil {
		return "err " + err.Error()
	}
	if u := t.Unix(); u < -9223372036 || u > 9223372036 {
		return "err mail: date out of range"
	}
	return "ok " + strconv.FormatInt(t.Unix(), 10)
}

// mailMessage is std/rt's read_message as the test prints it: the
// header's names sorted with their values in order, the body, and
// header("subject").
func mailMessage(msg []byte) string {
	r := textproto.NewReader(bufio.NewReader(bytes.NewReader(msg)))
	type kv struct{ k, v string }
	var hs []kv
	if len(msg) > 0 && (msg[0] == ' ' || msg[0] == '\t') {
		line, _ := r.ReadLine()
		return "err malformed initial line: " + textOf(line)
	}
	for {
		line, err := r.ReadContinuedLine()
		if line == "" {
			if err != nil && len(hs) == 0 {
				return "err " + err.Error()
			}
			break
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return "err malformed header line: " + textOf(line)
		}
		key := textproto.CanonicalMIMEHeaderKey(k)
		if key == "" {
			continue
		}
		hs = append(hs, kv{textOf(key), textOf(strings.TrimLeft(v, " \t"))})
	}
	body, _ := io.ReadAll(r.R)
	var keys []string
	vals := map[string][]string{}
	for _, h := range hs {
		if _, ok := vals[h.k]; !ok {
			keys = append(keys, h.k)
		}
		vals[h.k] = append(vals[h.k], h.v)
	}
	sort.Strings(keys)
	var w strings.Builder
	for _, k := range keys {
		w.WriteString(k + "=" + strings.Join(vals[k], "|") + ";")
	}
	b := "?"
	if utf8.Valid(body) {
		b = string(body)
	}
	subject := "-"
	if v, ok := vals["Subject"]; ok {
		subject = v[0]
	}
	w.WriteString("body=" + b + ";subject=" + subject)
	return w.String()
}

var mailPieces = []string{
	"a@b.c", "john.doe@example.com", "<x@y.z>", "Ann <ann@ex.com>", "\"Q, Name\" <q@ex.com>",
	"=?utf-8?q?J=C3=B6rg?= <j@ex.com>", "=?ISO-8859-1?Q?Andr=E9?=", "=?utf-8?b?w6k=?=", "=?unknown?q?x?=",
	"=?utf-8?q?bad=ZZ?=", "(comment)", "(nested (comment))", "(open", "g: a@b, c@d;", "empty:;",
	"[1.2.3.4]", "user@[127.0.0.1]", "a.@b", ".a@b", "a..b@c", "\"quoted local\"@x", "\\", "\"unclosed",
	"é@ü.example", "José <jose@ex.com>", " ", ",", ";", "<", ">", "@", ":", "x", "Bob", "@route:a@b",
	"a@b (Comment)", "<a@b> (c)", "\t", "a@b,", "=?UTF-8?Q?a_b?=", "\"\\\"esc\\\"\" <e@x>",
	// hostile shapes: control bytes, tabs, folds, nesting, a domain label
	// that runs long, a quoted local part with a newline in it
	"a\t@b", "\x01a@b", "a@b\x7f", "a\x00b@c", "a@b\nc", "a@b\rc", "\r\n a@b",
	"(a(b(c(d)))) a@b", "((unclosed)", "a@[::1]", "a@[fe80::1%25eth0]", "a@@b", "\"a\nb\"@c",
	"\">\" <a@b>", "\"<\"@b", "a@b.c.", "a@-b", "a@b_", strings.Repeat("a", 120) + "@ex.com",
	"a@" + strings.Repeat("b", 70) + ".com", "=?utf-8?b?!!?= <a@b>", "=?utf-8?q?=00?= <a@b>",
	"a@b (", "a@b )", "a@b (Comment) (x)", "\" \" <a@b>", "'a' <a@b>", "a@b <c@d>", "Name a@b",
}

func randomAddress(r *rand.Rand) string {
	var b strings.Builder
	for k := 0; k < 1+r.Intn(4); k++ {
		if k > 0 && r.Intn(3) == 0 {
			b.WriteString(" ")
		}
		b.WriteString(mailPieces[r.Intn(len(mailPieces))])
	}
	s := b.String()
	if r.Intn(4) == 0 && len(s) > 0 {
		// an ASCII byte replaced, so the input stays UTF-8
		bs := []rune(s)
		bs[r.Intn(len(bs))] = rune(" <>@,;:\"()[]\\.x"[r.Intn(15)])
		s = string(bs)
	}
	return s
}

func randomDate(r *rand.Rand) string {
	dows := []string{"", "Mon, ", "Fri, ", "Xyz, ", "Mon,"}
	days := []string{"1", "01", "31", "32", "0", "9"}
	months := []string{"Jan", "Feb", "Dec", "Foo", "jan"}
	years := []string{"2006", "06", "1997", "99", "70", "1", "20066", "9999", "0001"}
	times := []string{"15:04:05", "15:04", "23:59:60", "24:00:00", "9:05:06", "15:4:05", "15:04:05.123"}
	zones := []string{"-0700", "+0000", "GMT", "UT", "UTC", "EST", "MDT", "-07:00", "+1400", "-9999", "Z", "", "(MST)", "+0100 (CET)", "-0600 (MDT", "+0000 ((x))"}
	sep := []string{" ", "  ", "\t", "\r\n ", "\r"}
	s := dows[r.Intn(len(dows))] + days[r.Intn(len(days))] + " " + months[r.Intn(len(months))] + " " + years[r.Intn(len(years))] + sep[r.Intn(len(sep))] + times[r.Intn(len(times))] + " " + zones[r.Intn(len(zones))]
	if r.Intn(8) == 0 {
		s = " " + s
	}
	return s
}

func randomMessage(r *rand.Rand) []byte {
	names := []string{"From", "to", "Subject", "SUBJECT", "X-A", "x-a", "Date", "Bad Name", "", "Key ", "X_U", "é"}
	values := []string{"a", " b ", "", "=?utf-8?q?x?=", "v\r\n folded", "v\n\tfolded", "a:b", "\xff", "x\r"}
	nls := []string{"\r\n", "\n"}
	var b bytes.Buffer
	if r.Intn(10) == 0 {
		b.WriteString(" lead")
		b.WriteString(nls[r.Intn(2)])
	}
	for k := 0; k < r.Intn(5); k++ {
		n := names[r.Intn(len(names))]
		if r.Intn(10) == 0 {
			b.WriteString(n + nls[r.Intn(2)])
			continue
		}
		b.WriteString(n + ":" + values[r.Intn(len(values))] + nls[r.Intn(2)])
	}
	switch r.Intn(4) {
	case 0:
	case 1:
		b.WriteString(nls[r.Intn(2)])
	default:
		b.WriteString(nls[r.Intn(2)] + []string{"body", "line1\r\nline2\n", "\xfe\xff", ""}[r.Intn(4)])
	}
	return b.Bytes()
}

// randomMessageHostile builds a message from the bytes a hostile sender
// writes: header names and values with control bytes, NUL, DEL, obs-text
// bytes that are not UTF-8, tabs and long runs before and after the colon,
// folds in every position, CRLF and bare CR line ends, repeated names in
// different cases, and a body with 8-bit bytes or no blank line at all.
func randomMessageHostile(r *rand.Rand) []byte {
	names := []string{"From", "to", "Subject", "SUBJECT", "subject", "X-A", "x-a", "Bad Name", "", "Key ", "X_U", "é", "X\tA", "X\x01", "\x80", "a:"}
	values := []string{"", " ", "\t x", "a", " b ", "v\r\n folded", "v\n\tfolded", "a:b", "\xff", "\x00", "\x7f", "\xc3", "x\r", "x\n", strings.Repeat("v", 200), "=?utf-8?q?x?=", "=?bad?b?!!?=", "\xef\xbf\xbd"}
	nls := []string{"\r\n", "\n", "\r"}
	var b bytes.Buffer
	switch r.Intn(6) {
	case 0:
		// a leading fold is not a header
		b.WriteString([]string{" lead", "\t lead", "\r\n", "\r\n\r\n"}[r.Intn(4)])
	}
	for k, n := 0, r.Intn(6); k < n; k++ {
		nv := names[r.Intn(len(names))]
		switch r.Intn(8) {
		case 0:
			// no colon: a malformed header line
			b.WriteString(nv + nls[r.Intn(len(nls))])
		case 1:
			// an empty name
			b.WriteString(":" + values[r.Intn(len(values))] + nls[r.Intn(len(nls))])
		default:
			b.WriteString(nv + ":" + values[r.Intn(len(values))] + nls[r.Intn(len(nls))])
		}
	}
	switch r.Intn(4) {
	case 0:
		// no blank line: the body is not there
	case 1:
		b.WriteString(nls[r.Intn(len(nls))])
	default:
		b.WriteString(nls[r.Intn(len(nls))] + []string{"body", "line1\r\nline2\n", "\xfe\xff", "", "\x00\x01", "é日"}[r.Intn(6)])
	}
	return b.Bytes()
}

func mailCorpus() string {
	r := rand.New(rand.NewSource(1))
	lits := goTestStrings("net/mail/message_test.go")
	var o strings.Builder
	o.WriteString(header("net/mail ParseAddress, ParseAddressList, ParseDate, and ReadMessage as std/rt/mail.go maps them, on the strings of net/mail/message_test.go and seeded random ones"))
	o.WriteString("use std/mail\n\n")
	o.WriteString("/// (input, Go's result: ok NAME|ADDRESS|DISPLAY or err TEXT)\nconst ADDRESSES: List[(Str, Str)] = [\n")
	addrs := append([]string{}, lits...)
	for i := 0; i < 4000; i++ {
		addrs = append(addrs, randomAddress(r))
	}
	for _, s := range addrs {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(s), xq(mailAddress(s)))
	}
	o.WriteString("]\n\n/// (input, Go's result: ok NAME|ADDRESS;... or err TEXT)\nconst LISTS: List[(Str, Str)] = [\n")
	lists := append([]string{}, lits...)
	for i := 0; i < 1000; i++ {
		var parts []string
		for k := 0; k < 1+r.Intn(4); k++ {
			parts = append(parts, randomAddress(r))
		}
		lists = append(lists, strings.Join(parts, []string{",", ", ", " , ", ";"}[r.Intn(4)]))
	}
	for _, s := range lists {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(s), xq(mailList(s)))
	}
	o.WriteString("]\n\n/// (input, Go's result: ok UNIX_SECONDS or err TEXT)\nconst DATES: List[(Str, Str)] = [\n")
	dates := append([]string{}, lits...)
	for i := 0; i < 2000; i++ {
		dates = append(dates, randomDate(r))
	}
	for _, s := range dates {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(s), xq(mailDate(s)))
	}
	o.WriteString("]\n\n/// (message, Go's result: NAME=V|V;...body=BODY;subject=S, or err TEXT)\nconst MESSAGES: List[(Bytes, Str)] = [\n")
	var msgs [][]byte
	for _, s := range lits {
		if strings.Contains(s, ":") && strings.Contains(s, "\n") {
			msgs = append(msgs, []byte(s))
		}
	}
	for i := 0; i < 3000; i++ {
		if i%2 == 0 {
			msgs = append(msgs, randomMessage(r))
		} else {
			msgs = append(msgs, randomMessageHostile(r))
		}
	}
	for _, m := range msgs {
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(m), xq(mailMessage(m)))
	}
	o.WriteString("]\n")
	o.WriteString(mailTests)
	return o.String()
}

const mailTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

// Each test pins the rows it needs first: a regeneration that loses rows (a
// changed seed, a table cut short) fails here instead of quietly comparing
// less. The seeded rows alone are the floor; Go's own test strings add more.
test "parse_address is Go's" {
  expect(ADDRESSES.len() >= 4000)
  var bad: List[(Str, Str, Str)] = []
  for (s, want) in ADDRESSES {
    let got = match mail.parse_address(s) {
      Ok(a) => "ok ${a.name}|${a.address}|${a.display()}"
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "parse_address_list is Go's" {
  expect(LISTS.len() >= 1000)
  var bad: List[(Str, Str, Str)] = []
  for (s, want) in LISTS {
    let got = match mail.parse_address_list(s) {
      Ok(xs) => "ok " + xs.map(fn(a) { "${a.name}|${a.address}" }).join(";")
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "parse_date is Go's" {
  expect(DATES.len() >= 2000)
  var bad: List[(Str, Str, Str)] = []
  for (s, want) in DATES {
    let got = match mail.parse_date(s) {
      Ok(t) => "ok ${t.unix()}"
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

fn shape(m: mail.Message) -> Str {
  var keys: List[Str] = []
  for (k, _) in m.headers {
    if !keys.contains(k) {
      keys.push(k)
    }
  }
  var w = StrBuilder.new()
  for k in keys.sort() {
    let vs = m.headers.filter(fn(h) { h.0 == k }).map(fn(h) { h.1 })
    w.write("${k}=${vs.join("|")};")
  }
  w.write("body=${m.body.to_str() ?? "?"};subject=${m.header("subject") ?? "-"}")
  w.freeze()
}

test "read_message is Go's" {
  expect(MESSAGES.len() >= 3000)
  var bad: List[(Bytes, Str, Str)] = []
  for (m, want) in MESSAGES {
    let got = match mail.read_message(m) {
      Ok(x) => shape(x)
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((m, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
