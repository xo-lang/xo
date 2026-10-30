# `use go` coverage: Go function values as Xo functions, 2026-10-29

The survey set and method of usego-coverage-2026-10-29b.md (47 Go std packages and 40 third-party
modules, one project, `go run ./cmd/xo-gobind --dir PROJ --json PKG...`, Go 1.26.1, darwin/arm64).
Before: main after function-typed field setters (binding format 17, usego-coverage-2026-10-29c.md).
After: a Go function value given to Xo (a result, a field or variable read) of an unnamed function
type is an Xo function `uses go`; a named function type stays a handle.

| set | functions + methods before | after | variables before | variables after |
|---|---|---|---|---|
| std | 1838/2102 (87.4%) | 1842/2102 (87.6%) | 109/117 | 109/117 |
| third party | 12345/12650 (97.6%) | 12367/12650 (97.8%) | 372/380 | 374/380 |

Fields count as bound by their setters (format 17), so their new getters do not move the field
column. 99 skip entries were resolved. Getters that stay skipped: a function value with a
`context.Context` parameter (http.Transport.DialContext, go-redis's Dialer and OnConnect), several
results (logrus's CallerPrettyfier, go-redis's CredentialsProvider: the next push, tuples), a
variadic one (lo.Assert), and a `uintptr` parameter (zerolog's CallerMarshalFunc).

A first build that also gave named function types without methods the function value form lost 14
third-party functions, all functional options (zstd's `with_concurrent_blocks`, goldmark's
`with_parser`): they must go back to Go (gotchas/gobind-named-func-types-round-trip.md).
