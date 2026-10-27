package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/textproto"
	"os"
	"sort"
	"strings"
	"time"
)

// std/http's StatusError and the server's rendering of a handler's error
// (std.md 4.2), which no corpus reached: a handler returns Err(e) and the
// server answers e.status() and e.display() when E implements
// http.StatusError, or a 500 with "internal error" when it does not. The
// switch is rt.status_of, which resolves the implementation, so the
// interface's own methods are reached only by a call through a
// http.StatusError typed value, which is what the first test does.
//
// The cookie section is the other half: parse_cookie and parse_set_cookie
// (Go's ParseCookie and ParseSetCookie), each cookie's display (Go's
// Cookie.String), and CookieErr's display on a line Go refuses. The 2026-10-26
// audit listed all three as reached by no edge.
//
// The header and cookie tables carry hand written shapes and seeded random
// ones: names that are not tokens, control bytes, multi-byte text, obs-fold
// text in a value, and cookie lines built from every part of Go's grammar.
// net/http's own Header, ParseCookie and ParseSetCookie answer each case.
//
// The struct-literal table is the other reading of a header map: a map a
// program wrote as a literal keeps whatever case it wrote, so the documented
// case-insensitive read (std.md 4.3) answers where Go's Header.Get, which
// canonicalizes the lookup name and reads one key, misses. Each row records
// both answers and which relation it is, so the difference is pinned rather
// than assumed.
func httpCorpus() string {
	var o strings.Builder
	o.WriteString(header("std/http's StatusError through the interface, Router.call's rendering of a handler error that carries a status, of one that does not, and of a middleware's (std.md 4.2), and header and cookie parsing and rendering against Go's Header, ParseCookie, ParseSetCookie and Cookie.String"))
	o.WriteString("use std/http\n")
	o.WriteString(httpTests)
	o.WriteString(httpHeaderTables())
	o.WriteString(literalHeaderTables())
	o.WriteString(httpCookieTables())
	return o.String()
}

// headerNameAlphabet and headerValueAlphabet are the pieces seeded header
// names and values are built from: the token bytes, the bytes that make a name
// not a token (Go keeps such a name as it is), control bytes, a multi byte
// rune, U+FFFD (what a Str holds where the bytes were not UTF-8), and in a
// value the CRLF a fold starts with.
var headerNameAlphabet = []string{
	"a", "b", "c", "z", "A", "Z", "0", "9", "-", "-", "_", ".", "X",
	"Content", "Type", "Etag", "TE", "host", "Set-Cookie", "Authorization",
	" ", "\t", ":", "\x00", "\x7f", "\u0080", "é", "日", "�", "\"",
	"\\", ",", ";", "/", "=",
}

var headerValueAlphabet = []string{
	"a", "b", "Z", "0", "!", "~", " ", "\t", "\r", "\n", "\r\n ", "\r\n\t",
	"\x00", "\x1b", "\x7f", "\u0080", "é", "日本", "�", "\"", "\\",
	";", ",", "=", "%", "+", "/", ":", "<", ">", "text/plain", "charset=utf-8",
	"Wed, 09 Jun 2021 10:18:14 GMT",
}

func headerName(r *rand.Rand) string {
	var b strings.Builder
	for i, n := 0, r.Intn(4); i < n; i++ {
		b.WriteString(headerNameAlphabet[r.Intn(len(headerNameAlphabet))])
	}
	return b.String()
}

func headerValue(r *rand.Rand) string {
	var b strings.Builder
	for i, n := 0, r.Intn(6); i < n; i++ {
		b.WriteString(headerValueAlphabet[r.Intn(len(headerValueAlphabet))])
	}
	if r.Intn(6) == 0 {
		b.WriteString(strings.Repeat("x", r.Intn(40)))
	}
	return b.String()
}

// headerCaseFlip flips the case of each letter it is asked to.
func headerCaseFlip(s string, flip func() bool) string {
	var b strings.Builder
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z' && flip():
			b.WriteRune(c - 32)
		case c >= 'A' && c <= 'Z' && flip():
			b.WriteRune(c + 32)
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// headerLookup is the name a lookup asks for: the name itself, any case of it,
// its canonical form, or another name.
func headerLookup(r *rand.Rand, name string) string {
	switch r.Intn(10) {
	case 0, 1, 2, 3:
		return name
	case 4, 5, 6:
		return headerCaseFlip(name, func() bool { return r.Intn(2) == 0 })
	case 7:
		return strings.ToLower(name)
	case 8:
		return strings.ToUpper(name)
	default:
		return headerName(r)
	}
}

// goHeader is Go's answer for one header: the canonical names it holds,
// sorted, and Header.Get's answer for the lookup (the first value, or none).
func goHeader(name, value, lookup string) string {
	h := http.Header{}
	h.Set(name, value)
	var keys []string
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	got := "none"
	if vs, ok := h[textproto.CanonicalMIMEHeaderKey(lookup)]; ok && len(vs) > 0 {
		got = "ok " + vs[0]
	}
	return strings.Join(keys, "\x1f") + "|" + got
}

// goRepeatHeader is Go's answer for a name added several times: every value,
// then the view std.md 4.3 gives the Map (a repeated name is joined with ", ",
// but Set-Cookie keeps the first, decision 0288, where Go's Header.Get answers
// the first for every name).
func goRepeatHeader(names, values []string, lookup string) string {
	h := http.Header{}
	for i := range names {
		h.Add(names[i], values[i])
	}
	k := textproto.CanonicalMIMEHeaderKey(lookup)
	vs := h[k]
	if len(vs) == 0 {
		return "|none"
	}
	view := strings.Join(vs, ", ")
	if k == "Set-Cookie" {
		view = vs[0]
	}
	return strings.Join(vs, "\x1f") + "|ok " + view
}

// httpHeaderTables emits the header cases and what Go makes of them. A value
// with CRLF in it is obs-fold text: new_request stores a value as given (the
// server's reading of a folded field is http1's, a private module), so the case
// records the storage and the lookup, not the fold.
func httpHeaderTables() string {
	r := rand.New(rand.NewSource(63))
	var o strings.Builder
	o.WriteString("\n/// (name, value, lookup, Go's Header: canonical names sorted joined with U+001F, then Header.Get's answer)\nconst HEADERS: List[(Str, Str, Str, Str)] = [\n")
	for i := 0; i < 2000; i++ {
		name := textOf(headerName(r))
		value := textOf(headerValue(r))
		lookup := textOf(headerLookup(r, name))
		fmt.Fprintf(&o, "  (%s, %s, %s, %s),\n", xq(name), xq(value), xq(lookup), xq(goHeader(name, value, lookup)))
	}
	o.WriteString("]\n\n/// (names, values, lookup, Go's Header: every value joined with U+001F, then Get's first)\nconst REPEAT_HEADERS: List[(List[Str], List[Str], Str, Str)] = [\n")
	for i := 0; i < 600; i++ {
		first := textOf(headerName(r))
		nack := 1 + r.Intn(4)
		names := []string{first}
		values := []string{textOf(headerValue(r))}
		for j := 1; j < nack; j++ {
			names = append(names, textOf(headerLookup(r, first)))
			values = append(values, textOf(headerValue(r)))
		}
		lookup := textOf(headerLookup(r, first))
		fmt.Fprintf(&o, "  ([%s], [%s], %s, %s),\n", xqStrList(names), xqStrList(values), xq(lookup), xq(goRepeatHeader(names, values, lookup)))
	}
	o.WriteString("]\n")
	o.WriteString(httpHeaderTests)
	return o.String()
}

// xqStrList is xs as an Xo List[Str] literal.
func xqStrList(xs []string) string {
	var b strings.Builder
	for i, x := range xs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(xq(x))
	}
	return b.String()
}

// literalHeaderRows is the row count the generated test pins: a truncated
// table (a bad regeneration, a hand cut row) fails on the count, not later.
const literalHeaderRows = 2000

// literalHeaderKeyAlphabet builds the keys a struct literal's headers map is
// written with: a canonical token, the same name in other cases (what a
// literal keeps, where Header.Set would canonicalize), names with a byte
// outside a token, an empty name, multi-byte text, and raw bytes that are not
// UTF-8 (textOf turns them into U+FFFD for both sides, decision 0259).
var literalHeaderKeyAlphabet = []string{
	"Content-Type", "content-type", "CONTENT-TYPE", "Content-type",
	"X-A", "x-a", "X-Foo", "Host", "host", "TE", "te", "Set-Cookie",
	"a b", " a", "a ", "\tType", "a:b", "a(b", "x-\x80", "é", "É", "日本",
	"", "\xff", "a\xffb", "\xc3", "x\xe6\x97",
}

// literalHeaderLookup is the name a row looks up: a stored key, any case of
// it, its canonical form, or another name entirely.
func literalHeaderLookup(r *rand.Rand, key string) string {
	switch r.Intn(10) {
	case 0, 1, 2, 3:
		return key
	case 4, 5:
		return headerCaseFlip(key, func() bool { return r.Intn(2) == 0 })
	case 6:
		return strings.ToLower(key)
	case 7:
		return strings.ToUpper(key)
	case 8:
		return textproto.CanonicalMIMEHeaderKey(key)
	default:
		return textOf(headerName(r))
	}
}

// literalXoAnswer is the answer std.md 4.3 gives for the map a struct literal
// wrote, the rule std/native/http.xo header_get implements: the value under
// the lookup's canonical name, else, for a lookup name that is a token, the
// value of the stored key that folds to it (the keys are fold-unique, so the
// scan's order never decides the answer), else none.
func literalXoAnswer(h map[string]string, lookup string) string {
	if v, ok := h[textproto.CanonicalMIMEHeaderKey(lookup)]; ok {
		return "ok " + v
	}
	if !isTokenName(lookup) {
		return "none"
	}
	for k, v := range h {
		if asciiFoldEqual(k, lookup) {
			return "ok " + v
		}
	}
	return "none"
}

// asciiFoldEqual is http1's equal_fold: equal bytes after ASCII lower, so a
// non-ASCII pair that Unicode would fold (é and É) is not folded here.
func asciiFoldEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if asciiLower(a[i]) != asciiLower(b[i]) {
			return false
		}
	}
	return true
}

func asciiLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

// literalGoAnswer is net/http's answer for the same literal map: Header.Get
// canonicalizes the lookup name and reads that key alone, so a map whose keys
// were never canonicalized answers none where the documented read answers the
// stored value. Get returns "" for a missing name and for an empty value; the
// map read here keeps the two apart, since the key a lookup finds is what
// this table compares and Get's answer otherwise follows it.
func literalGoAnswer(h http.Header, lookup string) string {
	if vs, ok := h[textproto.CanonicalMIMEHeaderKey(lookup)]; ok && len(vs) > 0 {
		return "ok " + vs[0]
	}
	return "none"
}

// isTokenName is Go's httpguts.ValidHeaderFieldName's byte set (tchar).
func isTokenName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '!' || c >= '#' && c <= '\'' || c == '*' || c == '+' || c == '-' ||
			c == '.' || c == '^' || c == '_' || c == '`' || c == '|' || c == '~') {
			return false
		}
	}
	return true
}

// literalHeaderTables emits the struct-literal rows: a headers map written as
// a literal (no canonicalization and no fields list, unlike new_request),
// a lookup, the documented answer, Go's answer for the same map, and the
// relation between them: "same" when Go agrees, "case" when the lookup is a
// token and only the documented case-insensitive read finds the stored key
// where Go's Header.Get misses.
func literalHeaderTables() string {
	r := rand.New(rand.NewSource(67))
	var o strings.Builder
	o.WriteString("\n/// (entries, lookup, std.md 4.3's answer, Go's Header.Get, same|case)\nconst LITERAL_HEADERS: List[(List[(Str, Str)], Str, Str, Str, Str)] = [\n")
	for i := 0; i < literalHeaderRows; i++ {
		ents := map[string]string{}
		var order []string
		for n, count := 0, 1+r.Intn(4); n < count; n++ {
			k := textOf(literalHeaderKeyAlphabet[r.Intn(len(literalHeaderKeyAlphabet))])
			// fold-unique keys: with two keys folding together and neither
			// canonical, the fold scan's answer would depend on map order
			dup := false
			for seen := range ents {
				if asciiFoldEqual(seen, k) {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			ents[k] = textOf(headerValue(r))
			order = append(order, k)
		}
		if len(order) == 0 {
			order = append(order, "Content-Type")
			ents["Content-Type"] = textOf(headerValue(r))
		}
		gh := http.Header{}
		var pairs []string
		for _, k := range order {
			gh[k] = []string{ents[k]}
			pairs = append(pairs, fmt.Sprintf("(%s, %s)", xq(k), xq(ents[k])))
		}
		lookup := textOf(literalHeaderLookup(r, order[r.Intn(len(order))]))
		xo := literalXoAnswer(ents, lookup)
		got := literalGoAnswer(gh, lookup)
		rel := "same"
		if xo != got {
			rel = "case"
		}
		fmt.Fprintf(&o, "  ([%s], %s, %s, %s, %s),\n", strings.Join(pairs, ", "), xq(lookup), xq(xo), xq(got), xq(rel))
	}
	o.WriteString("]\n")
	o.WriteString(literalHeaderTests)
	return o.String()
}

// literalHeaderTests is the Xo side: the map is built with put (the same Map
// a literal builds, with no canonicalization and no fields list), the request
// is a struct literal, and both answers are checked: header() must be the
// documented one, and it must be Go's whenever Go agrees and the differing
// shape (a token lookup over a literal key, which Go's Header.Get misses)
// whenever the row says so.
const literalHeaderTests = `
test "a struct literal's headers read as std.md 4.3 says, next to Go's Header.Get on the same map" {
  var bad: List[(List[(Str, Str)], Str, Str, Str, Str, Str, Str)] = []
  var seen = 0
  for (entries, lookup, want, go_want, rel) in LITERAL_HEADERS {
    var h: Map[Str, Str] = {}
    for (k, v) in entries {
      h.put(k, v)
    }
    let u = http.Url{scheme: "http", host: "h", port: None, path: "/", query: {}}
    let r = http.Request{method: "GET", path: "/", url: u, headers: h, body: b"", remote_ip: ""}
    let got = header_answer(r, lookup)
    let want_values = if want == "none" { "" } else { want[3..want.len()] }
    let got_values = r.header_values(lookup).join("\u{1f}")
    let ok = if rel == "same" {
      got == want && go_want == want && got_values == want_values
    } else {
      got == want && go_want == "none" && want.starts_with("ok ") && got_values == want_values
    }
    if !ok {
      bad.push((entries, lookup, want, go_want, rel, got, got_values))
    }
    seen += 1
  }
  expect(seen) == LITERAL_HEADERS.len()
  expect(LITERAL_HEADERS.len()) == 2000
  expect(first_bad(bad)) == []
}
`

// httpCookieTables emits the cookie header lines and what Go makes of them:
// each parsed cookie's String (joined with "|"), or the error's Error(), which
// is the text std/native/http.xo builds its CookieErr messages from.
func httpCookieTables() string {
	// ParseSetCookie logs a Domain it drops; the log is not corpus data
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	cookieLines := []string{
		"a=b", "a=b; c=d", "a=b; c=d; e=f", "a=b; a=c", " a = b ; c = d ",
		"a=", "=b", "a=b,c=d", "a=b; c", "a", "a=b; c=d; e", "a=\"b\"",
		"a=\"b\\\"c\"", "a=\"b", "a b=c", "a=b c", "$Version=1; a=b",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 GMT", "a=b; b=c;", ";;a=b",
	}
	setLines := []string{
		"a=b", "a=b; Path=/x", "a=b; Domain=example.com", "a=b; Secure",
		"a=b; HttpOnly", "a=b; SameSite=Lax", "a=b; SameSite=Strict",
		"a=b; SameSite=None", "a=b; Max-Age=3600", "a=b; Max-Age=0",
		"a=b; Max-Age=-1", "a=b; Expires=Wed, 09 Jun 2021 10:18:14 GMT",
		"a=b; Expires=Thu, 01 Jan 1970 00:00:00 GMT",
		"a=b; Expires=Sat, 25 Dec 2021 23:59:59 GMT",
		"a=b; Expires=Wednesday, 09-Jun-21 10:18:14 GMT",
		"a=b; Expires=Wed, 09-Jun-2021 10:18:14 GMT",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14",
		"a=b; Expires=Wed Jun  9 10:18:14 2021",
		// the weekday's name, in any case, and anything else
		"a=b; Expires=WED, 09 Jun 2021 10:18:14 GMT",
		"a=b; Expires=Xyz, 09 Jun 2021 10:18:14 GMT",
		"a=b; Expires=Wedn, 09 Jun 2021 10:18:14 GMT",
		"a=b; Expires=W, 09 Jun 2021 10:18:14 GMT",
		// the zone's name: three or four capitals read, a lowercase one, a
		// numeric one, a bare offset and five letters do not
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 UTC",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 CEST",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 GMT+8",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 gmt",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 123",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 Z",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 CESTD",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 +0000",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 -0000",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 +0100",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 +00:00",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 GMT0",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 UTC0",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 GMT+0",
		// Go's parseTimeZone side: "ChST" and "MeST" are the only lower case
		// zones; UTC takes exactly three bytes; GMT takes a signed hour of 23
		// or less; a leading sign is an hour of 23 or less alone; a fourth
		// capital must be T or WITA, a fifth must be T
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 ChST",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 MeST",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 UTC+8",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 UTCT",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 UTCDT",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 GMT-23",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 GMT+24",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 GMTX",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 GMTT",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 +05",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 -7",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 +0023",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 +24",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 ABCD",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 WITA",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 AKDST",
		"a=b; Expires=Wed, 09 Jun 2021 10:18:14 AB€",
		// Go's month lookup folds ASCII case and reads the three letter names
		// only: "JUN" and "jun" read, "June" leaves "e" and does not
		"a=b; Expires=Sun, 29 JUN 2021 22:46:55 -0000",
		"a=b; Expires=Sun, 29 jun 2021 22:46:55 GMT",
		"a=b; Expires=Sun, 29 juN 2021 22:46:55 +0000",
		"a=b; Expires=Sun, 29 June 2021 22:46:55 GMT",
		"a=b; Path=/x; Domain=example.com; Secure; HttpOnly; SameSite=Lax; Max-Age=3600",
		"\"a\"=b", "a=\"b\"", "a=b; Path", "a", "=b", "a=b; Max-Age=abc",
		"a=b; Domain=bad_domain", "a=b; Path=/p; Path=/q", "a=b; Unknown=1",
		"a=b; Partitioned", "a=b; SameSite=bogus",
		// a date outside an Xo Time (about 1678 to 2262) has no Expires:
		// native's time.instant refuses it, and std/rt refuses it too
		"a=b; Expires=Wed, 16 Dec 9999 09:51:07 GMT",
		"a=b; Expires=Wed, 16 Dec 9999 09:51:07 +0000",
		"a=b; Expires=Sat, 30 Dec 1677 12:00:00 GMT",
		"a=b; Expires=Thu, 01 Jan 1600 00:00:00 GMT",
		"a=b; Expires=Wed, 16 Dec 2263 09:51:07 GMT",
		// the ends themselves are held
		"a=b; Expires=Fri, 01 Jan 1678 00:00:00 GMT",
		"a=b; Expires=Thu, 10 Apr 2262 00:00:00 GMT",
	}
	cookieLines = append(cookieLines, seededCookieLines()...)
	setLines = append(setLines, seededSetCookieLines()...)
	var o strings.Builder
	o.WriteString("\n/// (Cookie header value, Go's ParseCookie: each cookie's String joined with \"|\", or the error)\nconst COOKIE_LINES: List[(Str, Str)] = [\n")
	for _, line := range cookieLines {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(line), xq(goParseCookie(line)))
	}
	o.WriteString("]\n\n/// (Set-Cookie value, Go's ParseSetCookie: the cookie's String, or the error)\nconst SET_COOKIE_LINES: List[(Str, Str)] = [\n")
	for _, line := range setLines {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(line), xq(goParseSetCookie(line)))
	}
	o.WriteString("]\n")
	o.WriteString(httpCookieTests)
	return o.String()
}

// goParseCookie is Go's ParseCookie as the corpus records it.
func goParseCookie(line string) string {
	cs, err := http.ParseCookie(line)
	if err != nil {
		return err.Error()
	}
	var parts []string
	for _, c := range cs {
		parts = append(parts, c.String())
	}
	return strings.Join(parts, "|")
}

// timeMinSecs and timeMaxSecs are std/rt/timecivil.go's ends of an Xo Time
// (int64 nanoseconds, about 1677-09-21 to 2262-04-11), one second inside the
// partial end seconds so a nanosecond count is always representable.
const (
	timeMinSecs = math.MinInt64/1_000_000_000 + 1
	timeMaxSecs = math.MaxInt64/1_000_000_000 - 1
)

// goParseSetCookie is Go's ParseSetCookie as the corpus records it, with one
// Xo deviation: a date outside Time's range has no Expires in Xo, since
// native's time.instant refuses it and std/rt refuses it too (Go's own
// time.Time is wider, so the date is dropped from the oracle's answer).
func goParseSetCookie(line string) string {
	c, err := http.ParseSetCookie(line)
	if err != nil {
		return err.Error()
	}
	if !c.Expires.IsZero() {
		if sec := c.Expires.Unix(); sec < timeMinSecs || sec > timeMaxSecs {
			c.Expires = time.Time{}
		}
	}
	return c.String()
}

// cookieNameAlphabet and cookieValueAlphabet are the pieces the seeded cookie
// lines are built from: plain and reserved names, names with a space, a quote
// or a non-token byte, multi-byte text, U+FFFD, and values quoted, escaped,
// padded, control or empty.
var cookieNameAlphabet = []string{
	"a", "b", "c", "key", "session", "x", "A", "$Version", "$Path",
	"$Domain", "a b", "a=b", "a\tb", "a\"b", "é", "日本", "�", "",
	"__Host-x", "-x-", "a,b", "a;b", "Max-Age",
}

var cookieValueAlphabet = []string{
	"b", "", "v", "\"b\"", "\"a\\\"b\"", "v v", "é", "日本", "a;b",
	"a,b", "a=b", "\x00", "\x7f", "\u0080", "a\tb", "\\", "\"unterminated",
	"  padded  ", "\"\"", "a\"b", "%20", "again",
}

func cookieName(r *rand.Rand) string {
	return cookieNameAlphabet[r.Intn(len(cookieNameAlphabet))]
}

func cookieValue(r *rand.Rand) string {
	return cookieValueAlphabet[r.Intn(len(cookieValueAlphabet))]
}

// seededCookieLines are 2000 seeded Cookie header values: one to four pairs
// separated by every separator Go splits on, sometimes a bare token, sometimes
// a trailing separator.
func seededCookieLines() []string {
	r := rand.New(rand.NewSource(61))
	seps := []string{"; ", ";", ",", " ", "\t", ";;", ";  "}
	lines := make([]string, 0, 2000)
	for i := 0; i < 2000; i++ {
		var b strings.Builder
		for j, n := 0, r.Intn(5); j < n; j++ {
			if j > 0 {
				b.WriteString(seps[r.Intn(len(seps))])
			}
			if r.Intn(8) == 0 {
				b.WriteString(cookieName(r))
				continue
			}
			b.WriteString(cookieName(r))
			b.WriteString("=")
			b.WriteString(cookieValue(r))
		}
		if r.Intn(10) == 0 {
			b.WriteString(seps[r.Intn(len(seps))])
		}
		lines = append(lines, textOf(b.String()))
	}
	return lines
}

// seededSetCookieLines are 2000 seeded Set-Cookie values: a pair and zero to
// four attributes of every shape Go reads, an Expires in any of the three date
// shapes with any weekday, month and zone spelling.
func seededSetCookieLines() []string {
	r := rand.New(rand.NewSource(62))
	lines := make([]string, 0, 2000)
	for i := 0; i < 2000; i++ {
		var b strings.Builder
		b.WriteString(cookieName(r))
		b.WriteString("=")
		b.WriteString(cookieValue(r))
		for j, n := 0, r.Intn(5); j < n; j++ {
			b.WriteString("; ")
			switch r.Intn(12) {
			case 0, 1:
				b.WriteString("Path=")
				b.WriteString([]string{"/", "/x", "/a/b", "", "/x y", "/é", "/x;y", "\"/q\""}[r.Intn(8)])
			case 2:
				b.WriteString("Domain=")
				b.WriteString([]string{"example.com", "sub.example.com", ".example.com", "not a domain", "bad_domain", "é.com", "", "1.2.3.4", "[::1]"}[r.Intn(9)])
			case 3, 4:
				b.WriteString("Expires=")
				b.WriteString(cookieDate(r))
			case 5:
				b.WriteString("Max-Age=")
				b.WriteString([]string{"0", "1", "-1", "3600", "999999999999999999999", "abc", "", "+5"}[r.Intn(8)])
			case 6:
				b.WriteString([]string{"Secure", "HttpOnly", "Partitioned", "Secure=1", "HttpOnly=x"}[r.Intn(5)])
			case 7:
				b.WriteString("SameSite=")
				b.WriteString([]string{"Lax", "Strict", "None", "bogus", "", "lax"}[r.Intn(6)])
			case 8:
				b.WriteString([]string{"Priority=High", "SameParty", "Unknown=1", "Path", "Domain=", "Max-Age"}[r.Intn(6)])
			case 9:
				b.WriteString(cookieName(r))
			default:
				b.WriteString("Path=")
				b.WriteString([]string{"/x", "/y"}[r.Intn(2)])
			}
		}
		lines = append(lines, textOf(b.String()))
	}
	return lines
}

// cookieDate is an Expires value in Go's cookie TimeFormat, its RFC 850 shape,
// or its ANSI C shape, with a weekday, month and zone that may not be Go's.
func cookieDate(r *rand.Rand) string {
	wd := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun", "WED", "wed", "Wednesday", "Xyz", "Wedn", "W", "", "Mo"}
	mo := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec", "JUN", "junk", ""}
	zone := []string{"GMT", "UTC", "gmt", "CEST", "CESTD", "GMT+8", "GMT-8", "GMT+0", "GMT-23", "GMT+24", "GMTX", "GMTT", "+05", "-7", "+0023", "+0000", "-0000", "+0100", "+00:00", "GMT0", "UTC0", "ChST", "MeST", "ABCD", "WITA", "AKDST", "UTC+8", "UTCT", "UTCDT", "Z", "123", "é", "AB€", ""}
	year := []int{1970, 1999, 2000, 2021, 2026, 10000, 1600, 1677, 2263, 9999}[r.Intn(10)]
	day := 1 + r.Intn(31)
	h, m, s := r.Intn(24), r.Intn(60), r.Intn(60)
	switch r.Intn(3) {
	case 0:
		return fmt.Sprintf("%s, %02d %s %04d %02d:%02d:%02d %s", wd[r.Intn(len(wd))], day, mo[r.Intn(len(mo))], year, h, m, s, zone[r.Intn(len(zone))])
	case 1:
		return fmt.Sprintf("%s, %02d-%s-%02d %02d:%02d:%02d %s", wd[r.Intn(len(wd))], day, mo[r.Intn(len(mo))], year%100, h, m, s, zone[r.Intn(len(zone))])
	default:
		return fmt.Sprintf("%s %s %2d %02d:%02d:%02d %04d", wd[r.Intn(len(wd))], mo[r.Intn(len(mo))], day, h, m, s, year)
	}
}

const httpTests = `
enum RouteErr {
  Teapot
}
fn (e: RouteErr) status() -> Int { 418 }
fn (e: RouteErr) display() -> Str { "teapot" }

enum PlainErr {
  Broken
}
fn (e: PlainErr) display() -> Str { "broken" }

fn get(r: http.Router, path: Str) -> http.Response {
  let u = http.Url{scheme: "http", host: "h", port: None, path: path, query: {}}
  r.call(http.Request{method: "GET", path: path, url: u, headers: {}, body: b"", remote_ip: ""})
}

test "a handler's error with a status is a response with that status, through http.StatusError" {
  var r = http.router()
  r.handle("GET /teapot", fn(_q: http.Request) -> http.Response ! RouteErr { Err(RouteErr.Teapot) })
  let resp = get(r, "/teapot")
  expect(resp.status) == 418
  expect(resp.text()?) == "{\"error\":\"teapot\"}"
}

test "an error without a status is a 500 with the generic message, not its display" {
  var r = http.router()
  r.handle("GET /broken", fn(_q: http.Request) -> http.Response ! PlainErr { Err(PlainErr.Broken) })
  let resp = get(r, "/broken")
  expect(resp.status) == 500
  // the display is logged, not sent: a client learns nothing from it
  expect(resp.text()?) == "{\"error\":\"internal error\"}"
}

test "a middleware's error is rendered the same way" {
  var r = http.router()
  r.handle("GET /ok", fn(_q: http.Request) -> http.Response { http.text(200, "ok") })
  r.wrap(fn(_q: http.Request, _next: http.Next) -> http.Response ! RouteErr { Err(RouteErr.Teapot) })
  let resp = get(r, "/ok")
  expect(resp.status) == 418
  expect(resp.text()?) == "{\"error\":\"teapot\"}"
}

test "a value of the interface reads its status and its display" {
  let e: http.StatusError = RouteErr.Teapot
  expect(e.status()) == 418
  expect(e.display()) == "teapot"
}

test "a TlsErr displays the message it carries (std.md 4.6)" {
  // std.md 4.6 says "TlsErr their message", so the display is checked
  // against the error's own msg rather than a frozen text: the wording
  // belongs to std/tls and pinning it here would test that module's text
  let (shown, msg) = match http.tls_cert(b"not a certificate", b"not a key") {
    Ok(_) => ("accepted a garbage certificate", "")
    Err(e) => (e.display(), e.msg)
  }
  expect(msg.is_empty()) == false
  expect(shown) == msg
}
`

const httpHeaderTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

fn header_answer(r: http.Request, lookup: Str) -> Str {
  let got = match r.header(lookup) {
    Some(v) => "ok ${v}"
    None => "none"
  }
  got
}

test "a header name canonicalizes as Go's Header.Set does, and a lookup finds what Go's Get finds" {
  // the seeded rows alone: a regeneration that loses rows fails here
  // instead of quietly comparing less
  expect(HEADERS.len() >= 2000)
  var bad: List[(Str, Str, Str, Str, Str)] = []
  for (name, value, lookup, want) in HEADERS {
    var h: Map[Str, Str] = {}
    h.put(name, value)
    let r = http.new_request("GET", "http://h/", headers: h)
    let got = "${r.headers.keys().sort().join("\u{1f}")}|${header_answer(r, lookup)}"
    if got != want {
      bad.push((name, value, lookup, want, got))
    }
  }
  expect(first_bad(bad)) == []
}

test "repeated fields keep Go's values, and header() sees Go's first" {
  expect(REPEAT_HEADERS.len() >= 600)
  var bad: List[(List[Str], List[Str], Str, Str, Str)] = []
  for (names, values, lookup, want) in REPEAT_HEADERS {
    var r = http.new_request("GET", "http://h/", headers: {})
    var i = 0
    while i < names.len() {
      r = r.add_header(names[i], values[i])
      i += 1
    }
    let got = "${r.header_values(lookup).join("\u{1f}")}|${header_answer(r, lookup)}"
    if got != want {
      bad.push((names, values, lookup, want, got))
    }
  }
  expect(first_bad(bad)) == []
}

test "a struct literal's headers read the way the spec says" {
  let u = http.Url{scheme: "http", host: "h", port: None, path: "/", query: {}}
  let r = http.Request{method: "GET", path: "/", url: u, headers: {"content-type": "text/plain", "\tType": "tab"}, body: b"", remote_ip: ""}
  // a token name is case insensitive, whatever case a literal wrote
  expect(header_answer(r, "Content-Type")) == "ok text/plain"
  expect(header_answer(r, "CONTENT-TYPE")) == "ok text/plain"
  expect(r.header_values("CONTENT-TYPE").join(",")) == "text/plain"
  // a name with a non-token byte matches exactly, as Go matches it
  expect(header_answer(r, "\tTYPE")) == "none"
  expect(r.header_values("\tTYPE").is_empty())
  expect(header_answer(r, "\tType")) == "ok tab"
  // a Response reads the same way
  let resp = http.Response{status: 200, headers: {"x-a": "1", "a b": "2"}, body: b""}
  let xa = match resp.header("X-A") {
    Some(v) => v
    None => "none"
  }
  expect(xa) == "1"
  let ab = match resp.header("A B") {
    Some(v) => v
    None => "none"
  }
  expect(ab) == "none"
}
`
const httpCookieTests = `
fn cookie_lines_joined(cs: List[http.Cookie]) -> Str {
  cs.map(fn(c) { c.display() }).join("|")
}

test "parse_cookie is Go's ParseCookie and each cookie displays as Go's String" {
  expect(COOKIE_LINES.len() >= 2000)
  var bad: List[(Str, Str, Str)] = []
  for (line, want) in COOKIE_LINES {
    let got = match http.parse_cookie(line) {
      Ok(cs) => cookie_lines_joined(cs)
      Err(e) => e.display()
    }
    if got != want {
      bad.push((line, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}

test "parse_set_cookie is Go's ParseSetCookie and the cookie displays as Go's String" {
  expect(SET_COOKIE_LINES.len() >= 2000)
  var bad: List[(Str, Str, Str)] = []
  for (line, want) in SET_COOKIE_LINES {
    let got = match http.parse_set_cookie(line) {
      Ok(c) => c.display()
      Err(e) => e.display()
    }
    if got != want {
      bad.push((line, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}
`
