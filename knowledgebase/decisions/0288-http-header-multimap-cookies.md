# 0288: HTTP header fields with repeated names, and cookies

Status: Accepted (owner, 2026-10-20), as drafted, with three additions
the lead approved the same day: Cookie has `quoted: Bool = false` (Go's
Quoted, which String needs to round-trip); Go's Raw, RawExpires and
Unparsed are left out; a byte or domain Go drops with a log line is
dropped silently. Part 1 (2026-10-20, Xo Lang 3): Cookie, SameSite,
display, parse_cookie, parse_set_cookie, Request.cookie, cookies,
with_cookie. Part 2: the fields behind the Map (header_values,
add_header) and Response.with_cookie and cookies.

## Context

`Request.headers` and `Response.headers` are `Map[Str, Str]` (std.md
4.3): one value per canonical name. Incoming fields with the same name
are joined with ", " on both backends (std/native/llrt/http1.xo, the Go
backend's `strings.Join(vs, ", ")`), and a response sends one field per
name. That is right for most fields (RFC 9110 5.3 allows the join) and
wrong for `Set-Cookie`, which must not be joined (RFC 6265 3): a handler
cannot set two cookies, and a client reading two `Set-Cookie` fields
gets one value with a comma that is also inside each `Expires` date.
There is no Cookie type, so every program formats `Set-Cookie` and
parses `Cookie` by hand. Xo Lang Lib 1's std survey (2026-10-20) ranked
both as the top http gaps; decision 0276 says to port net/http's code
and tests.

Every handler and test reads `headers` as a Map today (map literals in
`http.Response{headers: {...}}`, `req.headers.get(...)`, goldens), so
the change must not break the Map.

## Decision (proposed): the spec text

std.md 4.3 gains:

```
fn (r: Request) header_values(name: Str) -> List[Str]    // every field of name, in order
fn (r: Request) add_header(name: Str, value: Str) -> Request
fn (r: Response) header_values(name: Str) -> List[Str]
fn (r: Response) add_header(name: Str, value: Str) -> Response

struct Cookie {
  name: Str
  value: Str
  path: Str = ""
  domain: Str = ""
  expires: Time? = None
  max_age: Int = 0          // seconds; 0 unset, < 0 delete now (Go's MaxAge)
  secure: Bool = false
  http_only: Bool = false
  same_site: SameSite = SameSite.Default
  partitioned: Bool = false
}
enum SameSite { Default, Lax, Strict, None }
fn (c: Cookie) display() -> Str                          // the Set-Cookie value, Go's Cookie.String
fn (r: Response) with_cookie(c: Cookie) -> Response      // add_header("Set-Cookie", c.display())
fn (r: Response) cookies() -> List[Cookie]               // the Set-Cookie fields that parse (Go's Response.Cookies)
fn (r: Request) cookie(name: Str) -> Cookie?             // the first Cookie pair of that name (Go's Request.Cookie)
fn (r: Request) cookies() -> List[Cookie]
fn (r: Request) with_cookie(c: Cookie) -> Request        // appends name=value to Cookie (Go's AddCookie)
```

- **The fields behind the Map.** A Request or Response keeps its header
  fields in order, hidden (like `params`): `header_values(name)` returns
  each field's value of that name (case insensitive), as received or as
  added. `headers` stays the Map view: a name's values joined with ", ",
  except `Set-Cookie`, whose Map value is its first field (joining
  cookies is never meaningful). `with_header` replaces every field of
  the name with one; `add_header` appends one. A struct literal or a
  direct change of `headers` sets that name's fields to the one value
  written: a Map entry that is not the join of the name's hidden fields
  wins, so code that only uses the Map works exactly as today.
- **Sending.** A response (and a client request) sends one field per
  value of `header_values`, in order: two `with_cookie` calls are two
  `Set-Cookie` fields. Over HTTP/2 the same, as separate header fields.
- **Receiving.** The server and the client keep each field; `headers`
  joins them as today, so its values do not change for existing code
  (only `Set-Cookie` in a client's response changes, from a broken join
  to the first field; `header_values` and `cookies()` give all).
- **Cookie formatting and parsing** are net/http's cookie.go ported:
  `display()` drops a name that is not a token and sanitizes values,
  paths, and domains as Go does (quoting a value with a space or comma,
  dropping invalid bytes), writes `Expires` in http.TimeFormat (GMT),
  `Max-Age=0` for a negative max_age, and `SameSite` only when set;
  `Request.cookies()` is readCookies (the `Cookie` fields split on `;`,
  invalid pairs skipped), `Response.cookies()` is readSetCookies (every
  attribute, unknown attributes ignored). Go's cookie_test.go cases are
  the tests, with net/http's texts where Go logs a dropped value.
- **Url.** Repeated query keys are already covered by
  `Request.query_all` and `url.Values` (decision 0279); the client sends a
  request's raw query when it has one (Lib 1's fix), so nothing more is
  needed here.

## Consequences

- No existing program changes behavior except a client reading a
  response with several `Set-Cookie` fields (before: one joined, broken
  value; after: the first field in `headers`, all in `header_values`).
- Each Request and Response carries a hidden field list; for the common
  case of one field per name the Map is still the store and the list is
  built only when `add_header` or a repeated incoming name needs it, so
  the server's per-request cost stays as measured (bench/perf/http).
- `xo vet` could later flag a `Set-Cookie` written through `headers` or
  `with_header` (only one survives); not part of this decision.

## Alternatives

- **A Header type replacing `Map[Str, Str]`** (Go's `http.Header`,
  `map[string][]string`): the most faithful, but it breaks every handler,
  test, and golden that builds or reads `headers` as a Map. A migration
  could keep a `headers_map()` view; rejected for the churn.
- **`Map[Str, List[Str]]`**: the same break, and every read of a single
  value gets clumsier.
- **Only `with_cookie`, no general multi-value API**: fixes Set-Cookie
  but not other repeated fields a proxy or client must see (`Via`,
  `Link`, `WWW-Authenticate`).
