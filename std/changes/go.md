# std/go changes

## 2026-10-29: feature: go.Any, Go's `any` in `use go` bindings

- Kind: feature
- Commit: this commit
- Found by: the Windows tester's `use go` coverage survey of v0.34.0
  (2026-10-29): `any` was 38% of std's skips and 70% of third party's
- Go reference: Go's empty interface; fmt.Sprintf, database/sql
  Rows.Scan, encoding/json Unmarshal; decision 0412
- Test: cmd/xo TestUseGoAny and TestUseGoAnyNoGoValue; internal/gobind

A `use go` binding now binds a function whose signature has Go's `any`:
the value is a `go.Any`, made with `go.any(x)` (implicit for the elements
of a list literal written for `...any`, as `fmt.sprintf("%d", [n])`) and
read with `as_int()`, `as_str()`, `as_list()`, `as_map()`, `as_handle[T]()`.
`go.out[T]()` is a Go pointer a Go function fills through `any`
(`json.unmarshal(data, v.any())`, then `v.get()`).
