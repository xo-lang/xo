# std/url changes

## 2026-11-14: perf: the URL type's display, escape and resolve_reference

- Kind: perf
- Commit: this push
- Found by: the decision 0616 perf kind on linux/arm64 host 1 at e2feedd59: parse 1.157, string 1.325, resolve 1.215 of Go, and escape 1.167 (bench/perf/url's main had grown the URL type's workloads, slowing its old ones)
- Go reference: net/url URL.String, escape, resolvePath, ResolveReference
- Change: the URL type's workloads move to bench/perf/urltype, so bench/perf/url is as it was; escape_in appends the runs it keeps and each `%XX` to one Str (no UTF-8 check of its result); resolve_path keeps each element as offsets in a list sized up front and writes them as slices; resolve_reference builds each result directly; display is one interpolation (presized by the compiler since the commit before)
- Measured: Mac, scripts/twincheck -time 9, Xo / Go: urltype parse 0.99, string 1.04, resolve 0.83; url escape 1.06, unescape 0.87, query 0.84, encode 0.69 (noisy; the Proven perf kind runs on host 1 after this push)
- Test: xo test --std-module=url (598); the urlfuzz and urltypefuzz corpora; twincheck output identical
- Affected: every backend
- Shipped in: not yet released

## 2026-11-14: fix: a path or fragment whose decoded bytes are not UTF-8 displays as Go's

- Kind: fix
- Commit: this push
- Found by: internal/stdtest/urltypefuzz_test.go (new, decision 0616's fuzz kind for the URL type): `http://example.com/%ff#f` through parse_request_uri displayed `%25FF%23f` where Go writes `%FF%23f`
- Go reference: net/url setPath, EscapedPath, setFragment, EscapedFragment
- Change: set_path and set_fragment keep the decoded bytes' escaped form in raw_path and raw_fragment when those bytes are not UTF-8 (the text as written when it is a valid encoding, else Go's escape of the bytes), so display, request_uri, resolve_reference and join_path write Go's text; the path itself still holds each invalid byte as its `%XX` escape (spec std 20.1)
- Measured: no change to the common path (no escapes)
- Test: TestUrlTypeFuzz (seed 7, 20,000 cases, arm64 and llvm) and its seed 1 corpus urltypefuzz on go, arm64 and llvm; xo test --std-module=url
- Affected: every backend, e2feedd59 only (the URL type's first push)
- Shipped in: not yet released

## 2026-11-14: feature: the URL type (Go's url.URL)

- Kind: feature
- Commit: this push
- Found by: the decision 0616 features axis (lead, 2026-11-14): std/url's net/url row was partial, with no URL type
- Go reference: net/url URL, Userinfo, Parse, ParseRequestURI, String, Redacted, ResolveReference, Query, RequestURI, Hostname, Port, JoinPath, and the escape modes of encoding_table.go (go1.26)
- Change: `Url`, `Userinfo` and `UrlErr`, with `parse`, `parse_request_uri` and `join_path` and the methods above (decision 0642), ported from url.go: Go's results and error texts, urlstrictcolons at its 1.26 default, decoded bytes that are not UTF-8 kept as their `%XX` escapes. Fast paths Go's String and setPath do not need: one buffer sized up front for display, a path with nothing to escape set without decoding, raw_path compared with path as it decodes, and resolve_reference's dot segments kept in a list and written once
- Measured: bench/perf/url gains parse, string and resolve workloads against Go's twin (scripts/twincheck -time 7 on the Mac, Xo / Go: parse 0.966, string 1.029, resolve 1.075 to 1.125 across runs; output identical); the Proven perf kind runs on linux/arm64 host 1 after this push
- Test: 529 cases from Go's url_test.go tables with Go's answers (scripts/genurltest) in url_test.xo, xo test --std-module=url on native and llvm; the Go backend's generated file builds (stdsync), and a probe of parse, display, resolve and join_path prints the same on arm64, the Go backend and Go
- Affected: every backend
- Shipped in: not yet released

## 2026-11-14: perf: parse_query finds each pair in one pass and unescapes only parts with an escape; encode sorts key indexes

- Kind: perf
- Commit: this push
- Found by: the decision 0616 std/url perf round (Xo Lang 3; XO_PROF on bench/perf/url query: the split list and a second unescape scan of every key and value)
- Go reference: net/url parseQuery (strings.Cut per pair, QueryUnescape per part) and Values.Encode (sorted keys)
- Change: parse_query counts the `&` separators and finds a `;` in one byte pass (the 10,000 parameter check first, GO-2026-4341), then walks the pairs with their first `=` in a second, with no list of parts, and calls the unescape only for a key or value with a `%` or `+`; encode of up to 16 keys orders key indexes by an insertion sort, reading each key's values at its index
- Measured: bench/perf/url on linux/arm64 host 1, --ab against 9799b18d0, 6 rounds each: query 0.81 (0.798 to 0.834), encode 0.729 (0.708 to 0.752); output unchanged
- Test: xo test --std-module=url on native and llvm; TestUrlFuzz (seed 2, arm64 and llvm) and TestUrlFuzzCorpusCurrent
- Affected: every backend
- Shipped in: not yet released

## 2026-11-14: perf, security: Values is an insertion-ordered Map of each key's values

- Kind: perf, security
- Commit: this push
- Found by: the decision 0616 std/url review (Xo Lang 3): get, all, has, add, set and remove scanned a key list, and add and parse_query copied a key's values to add one, so a handler reading every key of a query a peer fills with up to 10,000 parameters, or one key's 10,000 values, did quadratic work
- Go reference: net/url Values is a map[string][]string
- Form: up to 16 keys the key and value lists (bench/perf/url encode: a Map for a few keys was 1.083 of them on host 1), past 16 the Map
- Measured (Mac, native, 9,999 parameters): get of every key 65 ms to 0.15 ms (Go 0.17 ms); add of every key 78 ms to 0.56 ms (Go 4.5 ms); 9,999 values of one key parsed and added 189 ms to 0.54 ms; remove of every key 15 ms to 0.15 ms; parse_query and encode of distinct keys unchanged
- Test: std/url_test.xo "Values finds keys through add, set, remove and parse_query, in first-appearance order" and "many values of one key, parsed and added"; goldens http_form, http_tls, parse_edge_formats, url_basic, views_parsers on arm64, llvm and go; TestStdCorpora url and urlfuzz
- Affected: every backend; bounded by parse_query's 10,000 parameter limit (GO-2026-4341), so a cost per request, not unbounded
- Shipped in: not yet released

## 2026-11-13: fix: such an escape before a %25 zone fails with Go's ParseAddr error too

- Kind: fix
- Commit: this push
- Found by: the std/http 1M-case fuzz (decision 0616, TestHttpUrlFuzz seed 38 against go1.26.11)
- Go reference: net/url url.go parseHost unescapes the address before %25 and the zone after it, and netip.ParseAddr quotes both, taking "(at ...)" from the address alone
- Test: internal/stdtest/httpurlfuzz_test.go regressions `HTTP://[%e80::1%25eth0]/...`, `http://[fe80::%e9%25en%2525]/`, `http://[%c3%a9::1%25x]/`
- Shipped in: not yet released

## 2026-11-13: fix: an escape of a byte from 0x80 in an IP literal fails as Go's ParseAddr of that byte

- Kind: fix
- Commit: this push
- Found by: the std/http long fuzz (decision 0616, TestHttpUrlFuzz seed 11 against go1.26.11)
- Go reference: net/url url.go parseHost: the bracketed host is unescaped, and only %25 starts a zone, so `[fe80::1%A5eth0]` gives netip.ParseAddr("fe80::1\xa5eth0"), which fails
- Test: internal/stdtest/httpurlfuzz_test.go regressions `https://[fe80::1%A5eth0]/...`, `http://[%C3%A9::1]/`, `http://[::1%A5]/`
- Affected: native and LLVM backends; such a byte stayed escaped (decision 0259: not UTF-8), and netip read `%A5eth0` as a zone, so the URL parsed
- Shipped in: not yet released

## 2026-11-13: fix: a host's port starts at the last colon for every scheme but http and https, as Go 1.26.11

- Kind: fix
- Commit: this push
- Found by: the std/http long fuzz (decision 0616, TestHttpUrlFuzz seed 2 against go1.26.11)
- Go reference: net/url url.go parseHost (go.dev/issue/75223 and 78077): strict colon checking for http and https only; any other scheme takes the last colon as the start of the port, so comma-separated database host lists parse
- Test: internal/stdtest/httpurlfuzz_test.go regressions `a://h:8:/π#` (a host with an empty port) and `ftp://#fe80::1%25eth0]/π?a=%zz` (invalid port ":1%25eth0]")
- Affected: native and LLVM backends; before, only postgres and postgresql took the last colon (Go 1.26's rule), so `a://h:8:/` failed with invalid port ":8:"
- Shipped in: not yet released

## 2026-11-13: security: parse_query counts its separators before it splits

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2026-4341 (CVE-2025-61726)
- Go reference: net/url url.go parseQuery (the limit checked on a count of separators before any parameter is made), fixed in go1.25.6
- Test: std/url_test.xo "parse_query counts the separators before it splits"
- Affected: every release up to 0.50.1, all backends (the Go backend runs the generated port of parse_query, std/rt/std_gen_url.go, so Request.form there split first too; the regeneration fixes it)
- Shipped in: not yet released

`parse_query` split the whole text on `&` before it checked the 10,000 parameter limit, so a long query or form body (`Request.form`, up to the server's 64 MiB body limit) made one list entry per separator first, costing far more memory than its bytes (a denial of service class). The separators are now counted first and the limit fails before any part is made.

- Remote trigger: any client of a handler that calls `Request.form()` or `url.parse_query` on request data.

## 2026-11-11: internal: merged, escape's fast path in std/native/url_native.xo

- Kind: internal
- Commit: this push
- Found by: decisions 0588 and 0597 part 2
- Go reference: net/url url.go (go1.26.1)
- Test: `xo test --std-module=url` (native, LLVM); the url, httpurl, multipart and mime corpora on go, arm64, llvm; goldens url_basic, http_*, multipart*
- Timing: none (the code is the same); the portable escape measured 1.075 (BytesBuilder) and 1.041 (one write_be per escape) of this on host 1 and was not taken
- Shipped in: not yet released

std/url.xo holds the module and its tests (std/url_test.xo); escape is declared there with its body in std/native/url_native.xo (Go's PathEscape and QueryEscape on the Go backend). The Go backend now runs the port's parse_query and Values (generated into std/rt), where it ran net/url. std/native/url.xo is removed. Output unchanged.

## 2026-11-04: fix: url parsing keeps the path as written for the client's request target

- Kind: fix
- Commit: this push
- Found by: aws-19 (2026-11-04), see std/changes/http.md
- Go reference: net/url URL.setPath, RawPath
- Test: golden http_raw_path
- Timing: none
- Shipped in: not yet released

llrt/neturl's Url carries the path text before unescaping (`rawtext`);
http.new_request keeps it as Go's RawPath when it differs from the
default escaping of the decoded path. `url.parse` and `url.Url` are
unchanged.

## 2026-11-04: perf: escape, parse_query and Values.encode

- Kind: perf
- Commit: this push
- Found by: the new bench/perf/url twin (escape 1.55, parse_query 1.50, encode 1.21 of Go's net/url wall)
- Go reference: net/url escape (a shouldEscape table, one exact size buffer), ParseQuery
- Test: the url and httpurl corpora (arm64, LLVM, Go backend), `xo test --std-module=native/url`, twincheck bench/perf/url
- Timing: host 1 A/B against the parent: escape 0.695 (0.672-0.731), parse_query 0.788 (0.771-0.808), encode 0.894 (0.843-0.922), n=6 each
- Shipped in: not yet released

`query_escape` and `path_escape` classify bytes through a 256 byte table and write the result into an exact size buffer with `mem_copy` runs for the kept bytes. `parse_query` splits the query once and tests for `;` on the whole text. `Values.encode` sorts key indexes and reads each key's values by index. Results are unchanged.

## 2026-10-23: fix: a postgres URL keeps its comma-separated host list, as Go 1.26's parseHost

- Kind: fix
- Commit: this commit
- Found by: differential test against Go (internal/stdtest/testdata/corpus/httpurl, Go's url_test.go inputs)
- Go reference: net/url url.go parseHost, url_test.go urltests (go1.26.1, go.dev/issue/75223, 75859)
- Test: internal/stdtest/testdata/corpus/httpurl (TestStdCorpora, arm64 and LLVM)

`postgres://host1:1,host2:2,host3:3` failed with `invalid port ":1,host2:2,host3:3"
after host`. Go 1.26 keeps strict colon checking except for the postgres and
postgresql schemes, where the last colon starts the port. neturl.parse_host
now takes that case.

## 2026-10-22: perf: parse_query copies its keys and values of a long query

- Kind: perf
- Commit: 4438a0bf0
- Found by: memory probe (300 queries of 1 MiB, one kept key: 318 MB retained)
- Go reference: net/url url.go parseQuery (go1.26.1)
- Test: internal/llvm/testdata/golden/views_parsers (arm64, LLVM, go)

Slices are views (decision 0330), so a kept key or value of a long query
held all of it. Over 4 KiB each key and value is copied: 318 MB to 6 MB in
the probe; a shorter query keeps slices (bench/results/views-parsers-clone-2026-10-22.md).
## 2026-10-21: fix: unescape errors and parse_query's semicolon error as Go's

- Kind: fix
- Commit: 60c8372e
- Found by: differential test against Go (scripts/stdcorpus url: 1,500 escape texts, 1,200 queries)
- Go reference: net/url url.go EscapeError.Error, parseQuery (go1.26.1)
- Test: internal/stdtest/testdata/corpus/url

Natively, an escape cut inside a rune was quoted with `%XX` in the error
(`invalid URL escape "%#%C3"`); Go's strconv.Quote writes `"%#\xc3"`.
parse_query returned the first escape error even when a later pair held a
semicolon. Go reports `invalid semicolon separator in query` then,
because that error replaces any earlier one.

## 2026-10-20: port: std/url, Go's query and path escaping and url.Values

- Kind: port
- Commit: 3c9839e6
- Found by: survey
- Go reference: net/url/url.go (go1.26.1)
- Test: std/native/url_test.xo (Go's url_test.go tables); internal/llvm/testdata/golden/url_basic

New module: `query_escape`, `query_unescape`, `path_escape`,
`path_unescape`, and `Values` with `parse_query` and `encode`, with Go's
results and error texts on every backend. Bytes that decode to invalid
UTF-8 stay escaped, so the results are valid `Str`.
