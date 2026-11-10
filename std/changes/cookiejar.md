# std/cookiejar changes

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: internal/stdtest corpus cookiejar on go, arm64 and llvm

std/cookiejar.xo now holds the bodies of std/native/cookiejar.xo (removed), and the Go
backend compiles them from it as its own package. The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

## 2026-11-03: perf: no IP parse of host names; domain maps changed in place

- Kind: perf
- Commit: this push (std/cookiejar and compiler/ssa map.xo)
- Found by: the std timing rows (std-twin-rows12: 1.54 to 1.68 of Go) and XO_PROF on linux/arm64
- Go reference: net/http/cookiejar jar.go (isIP through net.ParseIP; entries changed in place)
- Test: internal/stdtest corpus cookiejar on arm64, LLVM and Go; bench/perf/cookiejar twincheck; test/codegen/maps
- Timing: get 1.415 of Go's wall (was 1.54), both 1.476 (was 1.681), set within noise of Go on linux/arm64 (bench/results/std-twin-cookiejar-2026-11-03.md)
- Shipped in: not yet released

A host name is no longer parsed as an IP address (a byte other than a
digit or a dot, and no colon, rules it out, as Go's ParseIP fails without
an error value), and a domain's cookie map is changed through Map.update
instead of read and written back. Map.update's closure no longer keeps
the value it was given for a fault (compiler/ssa), which had made every
write to a nested map copy it (gotchas/var-closure-kept-value-copies-map.md).

## 2026-11-03: feature: std/cookiejar, a cookie jar with Go's rules

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md: net/http/cookiejar); decision 0475 (Accepted, owner 2026-11-03)
- Go reference: net/http/cookiejar (go1.26.1, jar.go), BSD-style license
- Test: internal/stdtest/testdata/corpus/cookiejar (100 seeded sequences of SetCookies and Cookies, 12 hosts incl. IPv6, localhost and a trailing dot, hostile Domain and Path attributes, Max-Age and Expires, Secure, compared with the real package; handwritten host/domain, Secure and path order, expiry by time, clear and handle sharing) on arm64, LLVM and the Go backend; bench/perf/cookiejar twincheck against the real package
- Timing: bench/results/std-twin-rows12-2026-11-03.md: 1.67 (set), 1.54 (get) and 1.68 (both) of Go's wall, peak memory 0.22; not Proven, profile follows

`cookiejar.new`, `Jar.set_cookies`, `Jar.cookies`, `Jar.clear` (spec/std.md 59).
