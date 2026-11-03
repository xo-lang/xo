# 0475: std/cookiejar, a cookie jar for the HTTP client (proposal)

Status: Proposed

## Context

std-coverage.md lists net/http/cookiejar as planned ("with client cookies"). A client that logs in and then
calls an API (a scraper, a test driver, a service talking to a cookie session API) must keep Set-Cookie values
and send the right ones back: Go's `http.Client{Jar: jar}` does this with `cookiejar.New`. Xo has the
`http.Cookie` type (std/http 4.3, decision 0288, Set-Cookie parsing and `Request.with_cookie`) but no store, and
the client keeps no state between requests.

## Options

1. `std/cookiejar`, a store with Go's matching rules (RFC 6265 section 5: domain match, path match, host only
   cookies, expiry and Max-Age, Secure, the default path), without the HTTP client wiring:

   ```xo
   struct Jar {}                                    // a handle shared by copies, with its own lock (as Mutex)
   fn new() -> Jar
   fn (j: Jar) set_cookies(url: Url, cookies: List[http.Cookie], now: Time)    // Go's SetCookies
   fn (j: Jar) cookies(url: Url, now: Time) -> List[http.Cookie]               // Go's Cookies: those to send, sorted as Go
   fn (j: Jar) clear()
   ```

   The time is a parameter (a test passes a fixed instant; a caller passes `os.clock.now()`). The public
   suffix list is not consulted (Go's default is `nil` too); a jar therefore accepts a cookie for `com`: the
   caller who needs that protection passes a list later (not in this ADR). Entries are kept per
   registrable host key as Go does, with Go's limit of one cookie per (name, domain, path) and its ordering
   (longer paths first, then earlier creation).
   Cost: std/cookiejar.xo, a native implementation (about 200 lines, pure), a Go side as for other
   handle types, spec/std.md section, std/changes, corpus (Go's cookiejar tests ported: the jar_test.go
   tables of domains, paths, expiry, host-only and the default path, run against real net/http/cookiejar for
   seeded sequences), a timing row against Go's.
2. Also wire it into `http.Client`: a `jar: Jar?` option that adds Cookie headers and stores Set-Cookie on
   every response, redirects included. That is the use that matters, but it touches the client in std/http
   (Lang 3 owns it, but it is a separate ADR with the redirect-policy work, survey rank 1); this ADR keeps the
   jar usable by hand: `req.with_cookie` for each of `jar.cookies(url, now)`, and `jar.set_cookies(url,
   resp.cookies(), now)`.
3. Nothing: callers keep a `Map[Str, Cookie]` per host and get the matching wrong.

## Recommendation

Option 1 now; the client option as its own change once the jar exists and the redirect policy is decided.

## Consequences

A new public module (hence this ADR). `Jar` is a shared handle, like `Mutex`; it is not `Eq`, `Hash` or
`Json`. A persistent jar (saving to a file) is not included.

## Alternatives

Listed above (2, 3). Also rejected: a jar that reads the clock itself (not testable, needs the `clock`
capability on every call).
