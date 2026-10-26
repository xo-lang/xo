package main

import "strings"

// std/http's StatusError and the server's rendering of a handler's error
// (std.md 4.2), which no corpus reached: a handler returns Err(e) and the
// server answers e.status() and e.display() when E implements
// http.StatusError, or a 500 with "internal error" when it does not. The
// switch is rt.status_of, which resolves the implementation, so the
// interface's own methods are reached only by a call through a
// http.StatusError typed value, which is what the first test does.
func httpCorpus() string {
	var o strings.Builder
	o.WriteString(header("std/http's StatusError through the interface, and Router.call's rendering of a handler error that carries a status, of one that does not, and of a middleware's (std.md 4.2)"))
	o.WriteString("use std/http\n")
	o.WriteString(httpTests)
	return o.String()
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
