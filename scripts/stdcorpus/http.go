package main

import (
	"fmt"
	"net/http"
	"strings"
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
func httpCorpus() string {
	var o strings.Builder
	o.WriteString(header("std/http's StatusError through the interface, Router.call's rendering of a handler error that carries a status, of one that does not, and of a middleware's (std.md 4.2), and cookie parsing and rendering against Go's ParseCookie, ParseSetCookie and Cookie.String"))
	o.WriteString("use std/http\n")
	o.WriteString(httpTests)
	o.WriteString(httpCookieTables())
	return o.String()
}

// httpCookieTables emits the cookie header lines and what Go makes of them:
// each parsed cookie's String (joined with "|"), or the error's Error(), which
// is the text std/native/http.xo builds its CookieErr messages from.
func httpCookieTables() string {
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
		"a=b; Path=/x; Domain=example.com; Secure; HttpOnly; SameSite=Lax; Max-Age=3600",
		"\"a\"=b", "a=\"b\"", "a=b; Path", "a", "=b", "a=b; Max-Age=abc",
		"a=b; Domain=bad_domain", "a=b; Path=/p; Path=/q", "a=b; Unknown=1",
		"a=b; Partitioned", "a=b; SameSite=bogus",
	}
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

// goParseSetCookie is Go's ParseSetCookie as the corpus records it.
func goParseSetCookie(line string) string {
	c, err := http.ParseSetCookie(line)
	if err != nil {
		return err.Error()
	}
	return c.String()
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
`

const httpCookieTests = `
fn cookie_lines_joined(cs: List[http.Cookie]) -> Str {
  cs.map(fn(c) { c.display() }).join("|")
}

test "parse_cookie is Go's ParseCookie and each cookie displays as Go's String" {
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
