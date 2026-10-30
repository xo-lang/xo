# `use go` coverage: several results as an Xo tuple, 2026-10-29

The survey set and method of usego-coverage-2026-10-29b.md (47 Go std packages and 40 third-party
modules, one project, `go run ./cmd/xo-gobind --dir PROJ --json PKG...`, Go 1.26.1, darwin/arm64).
Before: main with Go pointers to basic types as `T?` (format 21) and Go function values as Xo
functions (format 22, usego-coverage-2026-10-29f.md). After: a callback or Go function value with
several results returns them as an Xo tuple (binding format 24).

| set | functions + methods before | after | fields before | fields after | interfaces Xo implements before | after |
|---|---|---|---|---|---|---|
| std | 1900/2102 (90.4%) | 1902/2102 (90.5%) | 446/461 | 446/461 | 56 | 62 |
| third party | 12411/12650 (98.1%) | 12452/12650 (98.4%) | 2080/2139 | 2088/2139 | 53 | 54 |

61 skip entries were resolved and none added. Most are samber/lo callbacks returning two to six
values (`slice_to_map`, `filter_map`, `unzip_by2` to `unzip_by6`, `try3` to `try6`); the others
are sync's `once_values`, bufio's `Scanner.split` (a `SplitFunc` returns three values), cobra's
completion functions, logrus's `CallerPrettyfier`, and go-redis's `CredentialsProvider` fields.
The interface column grows because an Xo type can now implement an interface whose methods return
several values: fmt.State, io.RuneReader and io.RuneScanner, net.PacketConn, http.Hijacker,
tls.ClientSessionCache.
