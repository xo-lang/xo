# `use go` coverage: function-typed fields and variables, 2026-10-29

The survey set and method of usego-coverage-2026-10-29b.md (47 Go std packages and 40 third-party
modules, one project, `go run ./cmd/xo-gobind --dir PROJ --json PKG...`, Go 1.26.1, darwin/arm64).
Before: main after 0416 step 4 (binding format 15, the "after" column of 2029b). After: a field or
package variable of a Go function type binds a setter taking an Xo callback (binding format 17).

Functions and methods are unchanged (std 1838 of 2102, third party 12345 of 12650): the change binds
fields and variables, counted from the stats of each binding:

| set | fields before | fields after | variables before | variables after |
|---|---|---|---|---|
| std | 421/461 | 446/461 | 108/117 | 109/117 |
| third party | 1955/2139 | 1985/2139 | 367/380 | 372/380 |

A function field counts as bound by its setter; its getter (a Go function value given to Xo) is
listed as skipped until the binder gives Go function values an Xo form.
