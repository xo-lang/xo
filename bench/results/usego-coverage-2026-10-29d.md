# `use go` coverage: Go pointers to basic types as `T?`, 2026-10-29

The survey set and method of usego-coverage-2026-10-29b.md (47 Go std packages and 40 third-party
modules, one project, `go run ./cmd/xo-gobind --dir PROJ --json PKG...`, Go 1.26.1, darwin/arm64).
Before: main after 0416 step 4 (binding format 15). After: decision 0419, a pointer to a basic type
is `T?`, a copy at the boundary, with the writes-through name check (binding format 21).

| set | functions + methods before | after | fields before | fields after |
|---|---|---|---|---|
| std | 1838/2102 (87.4%) | 1852/2102 (88.1%) | 421/461 | 421/461 |
| third party | 12345/12650 (97.6%) | 12363/12650 (97.7%) | 1955/2139 | 2050/2139 |

Writes-through skips (the parameter is skipped with "Go writes through ..., which Xo passes as a
copy"): 53, all real: 39 in sync/atomic (`add_int32(addr, ...)` and kin) and 14 flag `*Var`
functions and methods (`int_var(p, ...)`). A first run that also matched the names v, val and value
had 17 more, all false: zap's `Boolp(key string, val *bool) Field` and its kin read `*val` at once,
so those names were dropped from the check.
