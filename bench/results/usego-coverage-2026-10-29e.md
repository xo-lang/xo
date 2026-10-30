# `use go` coverage: methods of named basic types, 2026-10-29

The survey set and method of usego-coverage-2026-10-29b.md (47 Go std packages and 40 third-party
modules, one project, `go run ./cmd/xo-gobind --dir PROJ --json PKG...`, Go 1.26.1, darwin/arm64).
Before: main after 0416 step 4 (binding format 15). After: a value receiver method of a named basic
type is a package function `<type>_<method>` taking the value first (decision 0416 amendment,
binding format 20).

| set | functions + methods before | after |
|---|---|---|
| std | 1838/2102 (87.4%) | 1882/2102 (89.5%) |
| third party | 12345/12650 (97.6%) | 12371/12650 (97.8%) |

"Method of a named basic type" skips: std 46 to 2 (both pointer receiver methods, which Xo passes as
a copy), third party 30 to 0. Name collisions are unchanged (third party 12): no method name was
taken by a package function in the set.
