# std/http changes

## 2026-10-21: fix: a request's repeated Set-Cookie fields

- Kind: fix
- Commit: (filled in after the push)
- Found by: differential test against Go
- Go reference: net/http request parsing (readRequest, textproto ReadMIMEHeader) as the Go backend's fieldView shows it (go1.26.1)
- Test: internal/llvm/testdata/golden/http_header_fields (a request with two Set-Cookie fields); 467 raw requests (net/textproto and net/http parsing cases and 400 random header blocks) gave the same status, header Map, and body natively as on the Go backend

The native servers (HTTP/1.1 and HTTP/2) joined a request's repeated
Set-Cookie fields with ", " in `headers`; decision 0288 keeps the first,
as the Go backend and the client do (`header_values` gives each).

## 2026-10-21: fix: parse_url's host and fragment checks and error texts

- Kind: fix
- Commit: (filled in after the push)
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
- Commit: (filled in after the push)
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
