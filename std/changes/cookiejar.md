# std/cookiejar changes

## 2026-11-03: feature: std/cookiejar, a cookie jar with Go's rules

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md: net/http/cookiejar); decision 0475 (Accepted, owner 2026-11-03)
- Go reference: net/http/cookiejar (go1.26.1, jar.go), BSD-style license
- Test: internal/stdtest/testdata/corpus/cookiejar (100 seeded sequences of SetCookies and Cookies, 12 hosts incl. IPv6, localhost and a trailing dot, hostile Domain and Path attributes, Max-Age and Expires, Secure, compared with the real package; handwritten host/domain, Secure and path order, expiry by time, clear and handle sharing) on arm64, LLVM and the Go backend; bench/perf/cookiejar twincheck against the real package
- Timing: bench/results/std-twin-rows12-2026-11-03.md: 1.67 (set), 1.54 (get) and 1.68 (both) of Go's wall, peak memory 0.22; not Proven, profile follows

`cookiejar.new`, `Jar.set_cookies`, `Jar.cookies`, `Jar.clear` (spec/std.md 59).
