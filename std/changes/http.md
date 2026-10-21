# std/http changes

## 2026-10-21: feature: http.gzip_responses

- Kind: feature
- Commit: 4cddcc7b
- Found by: decision 0305 (approved by the owner, 2026-10-21)
- Go reference: none in Go's standard library; the rules of klauspost/compress gzhttp
- Test: internal/llvm/testdata/golden/http_gzip_responses

`r.wrap(http.gzip_responses())` compresses responses with gzip when the
request accepts it. It skips small bodies, compressed content types,
206s, and responses that already have a Content-Encoding. It adds
`Vary: Accept-Encoding`, weakens ETags, and compresses streamed bodies
through a Compressor that each flush reaches.
## 2026-10-21: feature: http.serve_content

- Kind: feature
- Commit: (filled in after the push)
- Found by: owner's request (2026-10-21)
- Go reference: net/http ServeContent, serveContent, checkPreconditions, scanETag, parseRange (go1.26.1)
- Test: internal/llvm/testdata/golden/http_serve_content (35 requests: ranges, conditional requests, HEAD, content types; the native backends match the Go backend's net/http byte for byte, the multipart boundary aside)

`http.serve_content(req, name, modtime, content, headers)` returns the
response Go's ServeContent writes: Range requests (one range as 206
with Content-Range, several as multipart/byteranges), If-Match,
If-Unmodified-Since, If-None-Match, If-Modified-Since and If-Range
against modtime and the ETag in `headers`, 416 for a range outside the
content, and the Content-Type from `headers`, the name's extension, or
the content (decision 0306).

## 2026-10-21: feature: Request.client_ip behind trusted proxies

- Kind: feature
- Commit: b37efc91
- Found by: survey (client addresses behind load balancers; with Xo Lang 3)
- Go reference: net/netip (Prefix.Contains, Addr.Unmap); RFC 7239; the walk of chi's and gorilla's proxy headers middleware
- Test: internal/llvm/testdata/golden/http_client_ip (every backend)

`client_ip(trusted, header)`: the peer, or the rightmost address of the
named proxy header outside the trusted prefixes, with only that header
read and the first untrusted hop ending the walk (None when it is hidden,
unknown, or does not parse; reviewed with Xo Lang 3).

## 2026-10-21: feature: the client keeps HTTP/1.1 connections alive

- Kind: feature
- Commit: 04ea3e47
- Found by: per-line profile of bench/perf/httpclient (Xo Lang 3)
- Go reference: net/http Transport (getConn, tryPutIdleConn, persistConn.readLoop, shouldClose, isReplayable, shouldRetryRequest; go1.26.1)
- Test: internal/llvm/testdata/golden/http_client_keepalive (connections a raw server accepted, request by request, as the Go backend's); TestHTTP1ClientDiff (457 responses, llvm, go, arm64)

Natively, an HTTP/1.1 connection (plain or TLS) goes back to a pool
after a response read in full that leaves it open, and the next request
to the same scheme, address and server name takes it: at most 2 idle
per host, 100 in all, 90 s idle, as Transport's defaults. A response
with Connection: close, an HTTP/1.0 one without keep-alive, a body that
ends at EOF, or a 101 closes it; a connection the server closed while
idle is dropped when taken, and a replayable request that met a dead
pooled connection is sent once more on a new one. The client bench's
HTTP/1.1 workloads against a keep-alive server: 1,119M to 520M
instructions for 3000 requests (decision 0303).

## 2026-10-21: fix: the native client reads responses as net/http does

- Kind: fix
- Commit: c9dad995
- Found by: differential test against Go
- Go reference: net/http ReadResponse, readTransfer (parseTransferEncoding, fixLength, fixTrailer, shouldClose), internal/chunked.go; net/textproto readMIMEHeader (go1.26.1)
- Test: internal/llvm/testdata/golden/http_client_responses (349 raw responses: net/http's parsing cases and 300 random header blocks; arm64, LLVM, and the Go backend agree); internal/llvm TestHTTP1ClientDiff's 457 random responses now agree with the Go backend on every one

The native HTTP/1.1 client was lenient where Go's is strict, and
reported other texts. It took a bad or negative Content-Length, two
different ones, or one like `2, 2`; a Transfer-Encoding other than
chunked; a field line without a colon, with an empty or invalid name,
or with a control byte; and kept only the first line of a folded field.
Each is now Go's error (`net/http: HTTP/1.x transport connection
broken: bad Content-Length "x"`, `unsupported transfer encoding:
"gzip"`, `malformed MIME header line: ...`), a folded field is joined
with a space, equal Content-Length fields read as one, a closing
HTTP/1.1 response has no `Connection` field (as net/http removes it),
and the body's errors (`unexpected EOF`, `invalid byte in chunk length`)
come without the request, as Go's body reads give them. An error text
keeps a zero byte of the line it quotes (it was cut there), a status
line's bytes are quoted as Go's %q (`"\xff"`), and a Location that
url.Parse rejects is Client.Do's `Get "URL": failed to parse Location
header ...`. On the Go backend, an error text quoting a field line with
bytes that are not UTF-8 now has U+FFFD for them (decision 0259), as
natively. TestHTTP1ClientDiff now holds every backend to the Go
backend's output on all 457 of its responses (testdata/http1_client.txt
is the Go backend's).

## 2026-10-21: speedup: Router matching without a string per path segment

- Kind: speedup
- Commit: 73612e39
- Found by: per-line callgrind of bench/perf/http (Xo Lang 3)
- Go reference: none
- Test: internal/llvm/testdata/golden/http_router_paths (124 method and path cases, the same answers as before the change)

Natively, a request path without escapes is matched against the routes
where it is: its segments' offsets are found once and each route's
literals compared in place, so no segment strings or unescaped copies
are made (a path with escapes is matched as before). The plain HTTP
server's router code took 177 fewer instructions a request (callgrind,
arm64).

## 2026-10-21: fix: a request's repeated Set-Cookie fields

- Kind: fix
- Commit: a6a1e92a
- Found by: differential test against Go
- Go reference: net/http request parsing (readRequest, textproto ReadMIMEHeader) as the Go backend's fieldView shows it (go1.26.1)
- Test: internal/llvm/testdata/golden/http_header_fields (a request with two Set-Cookie fields); 467 raw requests (net/textproto and net/http parsing cases and 400 random header blocks) gave the same status, header Map, and body natively as on the Go backend

The native servers (HTTP/1.1 and HTTP/2) joined a request's repeated
Set-Cookie fields with ", " in `headers`; decision 0288 keeps the first,
as the Go backend and the client do (`header_values` gives each).

## 2026-10-21: fix: parse_url's host and fragment checks and error texts

- Kind: fix
- Commit: b2ef88de
- Found by: differential test against Go
- Go reference: net/url Parse, parseHost, unescape (encodeHost, encodeZone), setFragment (go1.26.1)
- Test: std/native/http_test.xo (1,586 URLs: url_test.go's inputs and 1,500 seeded random ones, with the Go backend's results)

Natively, `parse_url` accepted 11 URLs Go rejects (a bad escape in the
fragment, which Go's setFragment checks) and rejected an IPv6 literal
with an escaped zone Go accepts (`[fe80::1%25%65%6e%30]`, RFC 6874), and
every bad host was `invalid host "..."` instead of Go's reason (`invalid
port ":0x" after host`, `missing ']' in host`, `invalid character " "
in host name`, netip's text for a bad IPv6 literal). The native host
parser is now a port of Go's parseHost; the same texts reach an x509
certificate's URI name errors.

## 2026-10-20: feature: header fields with repeated names; fix: Set-Cookie

- Kind: feature
- Commit: 33fec016
- Found by: survey
- Go reference: net/http Header.Add, Header.Values, SetCookie, Response.Cookies (go1.26.1)
- Test: internal/llvm/testdata/golden/http_header_fields (HTTP/1.1 and HTTP/2; arm64, LLVM, and the Go backend agree)

`header_values` and `add_header` on Request and Response, and
`Response.with_cookie` and `cookies` (decision 0288, part 2): several
fields of one name are sent and received each. A fix with it: a client
response with several `Set-Cookie` fields was one value joined with ", "
(which `Expires` dates also contain); `headers` now shows the first, and
`header_values` or `cookies()` give all. The HTTP/2 client also kept the
last of a repeated field in the Map instead of joining them.

## 2026-10-20: feature: cookies

- Kind: feature
- Commit: 54da082a
- Found by: survey
- Go reference: net/http/cookie.go Cookie.String, ParseCookie, ParseSetCookie, readCookies, Request.Cookie, Cookies, AddCookie (go1.26.1)
- Test: internal/llvm/testdata/golden/http_cookies (the inputs of Go's cookie_test.go tables; arm64, LLVM, and the Go backend's net/http agree); std/native/http_test.xo (Go's TestCookieSanitizeValue, TestCookieSanitizePath, writeSetCookiesTests' domains and names)

`http.Cookie` with `display()` (the Set-Cookie value), `parse_cookie`,
`parse_set_cookie`, and `Request.cookie`, `cookies`, and `with_cookie`
(decision 0288, part 1). Bytes and domains Go drops with a log line are
dropped silently.
## 2026-10-20: change: form() parses the Content-Type as Go does

- Kind: change
- Commit: 11afca47
- Found by: review (Xo Lang 3)
- Go reference: net/http/request.go parsePostForm, TestParseFormUnknownContentType (go1.26.1)
- Test: std/native/http_test.xo (go TestParseFormUnknownContentType); internal/llvm/testdata/golden/http_form

`req.form()` reads the media type with `mime.parse_media_type`, so a
malformed Content-Type (`text/plain; boundary=`) is now an error with
mime's text, where it was an empty form; a well formed one behaves as
before.

## 2026-10-20: port: Request.multipart_form

- Kind: port
- Commit: 4de48c2e
- Found by: survey
- Go reference: net/http/request.go ParseMultipartForm, multipartReader (go1.26.1)
- Test: internal/llvm/testdata/golden/multipart_basic

`req.multipart_form()` reads a `multipart/form-data` body as a
`multipart.Form` with Go's Content-Type checks and error texts (decision
0282).

## 2026-10-20: fix: the client sends a URL's query as given

- Kind: fix
- Commit: 361a427c
- Found by: survey
- Go reference: net/http/client.go, net/url URL.RawQuery (go1.26.1)
- Test: internal/llvm/testdata/golden/http_form (repeated keys and their order); internal/llvm/testdata/golden/http_tls (HTTP/2)

The client rebuilt a request's query from `url.query`, a map holding one
value per key, so `client.get("http://h/s?q=a&q=b")` sent only `q=a`. It
now sends the query as given unless `url.query` was changed.

## 2026-10-20: feature: Request.form and Request.query_all

- Kind: feature
- Commit: e2be349f
- Found by: survey
- Go reference: net/http/request.go ParseForm, PostForm; net/url URL.Query (go1.26.1)
- Test: std/native/http_test.xo (Go's TestParseFormQuery, TestParseFormQueryMethods, TestParseFormUnknownContentType, TestParseFormSemicolonSeparator); internal/llvm/testdata/golden/http_form

`req.form()` parses an `application/x-www-form-urlencoded` body into
`url.Values`, and `req.query_all(name)` returns every value of a repeated
query key; the server keeps each request's raw query for it, over HTTP/1.1
and HTTP/2.
