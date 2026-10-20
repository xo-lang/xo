# std/http changes

## 2026-10-20: feature: Request.form and Request.query_all

- Kind: feature
- Commit: (filled in after the push)
- Found by: survey
- Go reference: net/http/request.go ParseForm, PostForm; net/url URL.Query (go1.26.1)
- Test: std/native/http_test.xo (Go's TestParseFormQuery, TestParseFormQueryMethods, TestParseFormUnknownContentType, TestParseFormSemicolonSeparator); internal/llvm/testdata/golden/http_form

`req.form()` parses an `application/x-www-form-urlencoded` body into
`url.Values`, and `req.query_all(name)` returns every value of a repeated
query key; the server keeps each request's raw query for it, over HTTP/1.1
and HTTP/2.
