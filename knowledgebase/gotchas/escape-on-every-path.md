# Taking an address on one branch moves the value to the heap on all

Status: Observed (2026-10-08, std/rt allocation work on bench/perf/http)

Symptom: runtime helpers that look allocation free cost one heap
allocation per call, for every type, including the types their fast
path handles without the address: `JsonEnc` (a type switch on
`any(&v)`), `Own` and `Mutex.with_read` (`MarkP(&v)`), `mtable.setArr`
(`t.arr.Store(&a)` on its slow branch), and `Map_values_seq` (two
closures, one calling `MarkP(&v)`). Together they were about 70 of the
199 allocations of a `GET /users` with 64 users.

Cause: Go's escape analysis is flow insensitive. If `&v` reaches an
interface, an atomic pointer, or a function that leaks its argument on
any path, `v` is moved to the heap at function entry, whatever branch
runs. A generic shape function does not help: `markable[T]()` is a
runtime check, not a constant, so the compiler cannot drop the branch.

Fix: keep the address off the variable the fast path uses.

- Switch on the type with `any((*T)(nil)).(type)` and read the value
  through `*(*string)(unsafe.Pointer(&v))` (a local unsafe conversion
  does not escape); copy into `p := new(T)` only on the slow path
  (`std/rt/json.go` JsonEnc).
- For marking, `if markable[T]() { p := new(T); *p = v; MarkP(p); v = *p }`
  (`Own`).
- Publish a fresh header (`p := new([]E); *p = a`) instead of `&a`.
- Keep iterator constructors tiny so they inline and their closures stay
  on the stack (`Map_values_seq` calls `m.eachValue(yield)`).
- Encode list elements through `&l.s[i]` (already heap memory), and have
  generated code call `v.F.XoJsonEnc(tk, w)` directly instead of going
  through a generic helper.

Check with `go build -gcflags=-m ./std/rt` ("moved to heap: v") or pin
it with `testing.AllocsPerRun` (`std/rt/alloc_test.go`).
