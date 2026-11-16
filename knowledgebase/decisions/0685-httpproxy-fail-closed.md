# 0685: std/httpproxy fails the request on a proxy setting it cannot use (amends 0617 B)

Status: Accepted (owner, 2026-11-16; proposed by azure-1, 2026-11-16; the lead asked for it before the
code, after TestHttpproxyFuzz, task #115).

## Context

Decision 0617 B gave std/httpproxy Go's rules for HTTP_PROXY, HTTPS_PROXY
and NO_PROXY, with the type `fn(http.Url) -> http.Url?` for the chooser
(`Config.proxy_func`, `proxy_from_env`; None connects directly), which
`Client.with_proxy_for` takes. A proxy setting that does not parse as an
http or https URL is ignored, so the request connects directly. Under CGI
(REQUEST_METHOD set) an http request's HTTP_PROXY is ignored the same way.

TestHttpproxyFuzz, a differential against net/http's ProxyFromEnvironment,
showed what Go does with such settings, and it is not one rule:

- golang.org/x/net/http/httpproxy's `config.init` drops parseProxy's
  error (`if parsed, err := parseProxy(...); err == nil { ... }`), so a
  value that fails both of its parses ("http://%zz") connects directly in
  Go too.
- parseProxy's fallback (it puts "http://" before a value that does not
  parse, or parses with no scheme or host, and returns the first parse
  when that fails too) gives URLs no transport can dial: " proxy.test"
  and "host:x" give one with no host, "http://host:x" and "http://" one
  whose host is "http". The request then fails, by accident.
- Under CGI, Go's ProxyFunc returns an error for an http request with
  HTTP_PROXY set (golang.org/s/cgihttpproxy), so the request fails.

A setting that is set but unusable is a misconfiguration of a proxy the
deployment may require. Connecting directly then sends traffic around
that proxy without telling anyone: it fails open.

## Decision

1. A proxy setting that is set (not empty after trimming) but unusable
   fails every request it would have carried, with an error that names
   the setting and never echoes its value (a URL may hold credentials):
   `proxyconnect: HTTPS_PROXY is set but is not an http or https proxy
   URL`. Unusable: it does not parse as an http or https URL, by itself
   or with "http://" before it, or it parses with no host. NO_PROXY still
   applies first: a request it excludes connects directly, as with a
   good setting (Go's proxyForURL order).
2. Under CGI, an http request with HTTP_PROXY set fails with Go's text,
   `refusing to use HTTP_PROXY value in CGI environment; see
   golang.org/s/cgihttpproxy`, where it connected directly.
3. The chooser takes Go's shape, a result that can fail:
   - `Config.proxy_func() -> fn(http.Url) -> http.Url? ! Str`
   - `proxy_from_env(env: Env) -> (fn(http.Url) -> http.Url? ! Str) uses env`
   - `Rules.proxy_for(u: http.Url) -> http.Url? ! Str`
   - `http.Client.with_proxy_func(f: fn(http.Url) -> http.Url? ! Str) ->
     Client` (Go's Transport.Proxy): an Err fails the request as
     `proxyconnect: MESSAGE`, a ClientErr.Connect like a proxy that does
     not answer. `with_proxy_for` (None to connect directly, no error)
     and `with_proxy` stay as they are.
4. This is stricter than Go where x/net drops the parse error (point 1
   covers "http://%zz" too). std/httpproxy's doc lists it as a
   difference from Go, and TestHttpproxyFuzz models it: Go's answer for
   a set but unusable setting is taken as the error.

## Consequences

- Breaking: callers of `proxy_func` and `proxy_from_env` get a fallible
  function. `client.with_proxy_for(httpproxy.proxy_from_env(env))`
  becomes `client.with_proxy_func(httpproxy.proxy_from_env(env))`; code
  that calls the function itself handles the Err (`?`, or a match). The
  CHANGELOG has a Breaking line saying so.
- spec/std.md section 82 (and the with_proxy_func line in section 20's
  client) change with the code; decision 0617 B's text keeps its history
  and points here.
- Known callers in the repository: bench/perf/httpproxy_rules,
  internal/llvm/testdata/golden/log_sinks, the httpproxy corpus
  (scripts/stdcorpus/httpproxy.go), scripts/httpproxydiff and the
  httpproxy stress program; all change in the same commit.
- Tests: a std/httpproxy test that fails before (an unusable HTTPS_PROXY
  and a CGI HTTP_PROXY each fail the request; NO_PROXY still bypasses), a
  std/http golden for with_proxy_func's error, and TestHttpproxyFuzz.
