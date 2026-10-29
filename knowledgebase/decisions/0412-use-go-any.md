# 0412: `use go`: Go's `any` as a dynamic Go value (go.Any)

Status: Proposed (draft 1, 2026-10-29, Lang 1; owner approved writing it, the decision is the owner's)

## Context

`use go` (spec 4.4, the binder internal/gobind) skips every declaration whose signature has Go's `interface{}`/`any`
anywhere (skip category "interface{} or any"). The Windows tester's survey of 87 packages (47 std, 40 third party;
Xo 0.34.0, Go 1.27.0; their knowledgebase/findings/usego-coverage-2026-10-29.md) bound 80.5% of std's and 76.9% of
third-party functions and methods; `any` is the largest skip reason: 158 of std's 413 skips (38%) and 2,059 of third
party's 2,923 (70%). The skipped names are the ones applications reach for first: fmt.Sprintf/Printf/Errorf, database/sql
DB.Query/Exec/QueryRow and Rows.Scan, encoding/json and yaml Marshal/Unmarshal, logrus Info/WithField, zap's Any field,
testify's assert, samber/lo (also generic), redis Client.Get/Set's args, pgx Conn.Query/Exec.

Upper bound of what this decision can reach (the survey records one reason a skip; a function may also have a second):
std 1,708 + 158 = 1,866 of 2,121 (88.0%), third party 9,748 + 2,059 = 11,807 of 12,671 (93.2%). The measured "after"
comes with the first build (see Measurement).

## Decision

1. **The type.** Every binding that has an `any` names one Xo type, `go.Any` (std's `go` module, beside `go.Error`): an
   opaque handle to a Go `any` value (the adapter's `rt.GoAny{V any}`), one type for every package (as decision 0392's
   one Xo type per Go type: `any` is one Go type). A Go parameter or result of type `any`, `interface{}`, or a named
   empty interface becomes `go.Any`; `[]any` becomes `List[go.Any]`, `map[string]any` becomes `Map[Str, go.Any]`.

2. **Into Go: `go.any(x)`.** An explicit conversion, `go.any(x: T) -> go.Any` for T a bindable value type: Int, Int64,
   UInt64 and the sized ints, Float, Float32, Bool, Str, Bytes, Duration, Time, any `use go` handle (its Go value),
   List/Map of those (copied to []T / map[K]V of the Go type the binder maps them from), and an Option of those (None
   is Go's nil). Not implicit: a call site says it hands a value to Go's dynamic world, and overload-free resolution
   stays simple. Variadic `...any` (Sprintf, Printf, Query's args, logrus Info) is `List[go.Any]`; helpers spare the
   noise at the common sites: none in the first version (`fmt.sprintf("%d %s", [go.any(n), go.any(s)])`); a later
   `go.args(...)` form is an open question.

3. **Out of Go: checked conversions.** `go.Any` has methods `as_int() -> Int?`, `as_float() -> Float?`, `as_bool() ->
   Bool?`, `as_str() -> Str?`, `as_bytes() -> Bytes?`, `as_list() -> List[go.Any]?` ([]any or any slice),
   `as_map() -> Map[Str, go.Any]?` (map[string]any or any map with string keys), `as[T: GoHandle]() -> T?` for a
   bound handle type, `is_nil() -> Bool`, `type_name() -> Str` (Go's %T), and `display` (Go's %v). None on a mismatch,
   no implicit numeric widening except Go's int kinds to Int and float32 to Float. A `match` on go.Any is not added
   (no Xo sum type to match); `type_name()` plus the `as_*` cover a type switch.

4. **Pointer outputs (Scan(&x), Unmarshal(&v), Decode(&v)).** The same problem as the tester's bug 18 (Go writes into a
   caller's buffer): a Go function that takes `any` to fill. Bound as an *output form*: `go.out[T]() -> go.Out[T]`
   allocates a Go `*T` (T from the bindable set above, or `go.Any` for `*any`), it is passed where Go wants `any`
   (go.Out converts to go.Any), and `.get() -> T` reads it back after the call:
   `let id = go.out[Int](); let name = go.out[Str](); rows.scan([id.any(), name.any()])?; use(id.get(), name.get())`;
   `var v = go.out[go.Any](); json.unmarshal(data, v.any())?; v.get().as_map()`. The same form later carries bug 18's
   `Read(p []byte)` (go.out over a []byte of n bytes), which stays skipped until then.

5. **Equality and lifetime.** go.Any compares as Go's `==` on the interface (dynamic type and value; an uncomparable
   dynamic type, a slice or map, is unequal to everything but itself by identity, never a Go panic) and hashes
   consistently; it is a handle (spec 2.9), kept alive by the Xo value like every handle; a Go value reached from it
   lives while the handle does.

6. **What stays skipped.** Non-empty interfaces stay as today (named ones are handles already); generic functions and
   types (424 third party skips, decision territory of their own); `any` inside a struct field of a struct passed by
   value (bound as a handle's getter/setter returning go.Any is fine; an unnamed struct stays skipped); channels of any;
   callbacks with any parameters bind (go.Any in, go.Any out); `...interface{ M() }` stays skipped.

7. **Replay.** A function taking or returning go.Any records its go.Any results by Go's %#v text and replays them as
   opaque values that only display (`as_*` on a replayed value faults with "a replayed Go value"), unless the function is
   `replay rerun`.

## Spec 4.4 diff (draft)

- Types at the boundary: ...; `any` (`interface{}`) is `go.Any`, an opaque Go value: `go.any(x)` makes one from a
  bindable Xo value, `as_int()`, `as_str()`, ... read one back (None on another dynamic type), `type_name()` names its
  Go type; `...any` is `List[go.Any]`; a Go function that fills a value through `any` (`Scan(&x)`, `Unmarshal(&v)`)
  takes `go.out[T]()`'s `.any()` and the program reads `.get()` after the call (decision 0412).
- Exported names whose signatures cannot be mapped are skipped ...: generic ones, non-empty unnamed interfaces, ...

## Measurement (before building, and with the first build)

- Before: rerun the tester's survey here (87 packages, `xo bind --json`, tools/usego-survey/agg.js) on main; record
  per-reason skip counts in bench/results/usego-coverage-YYYY-MM-DD.md.
- After: the same, with the binder change; the target is std >= 87% and third party >= 90% (upper bounds 88.0 and
  93.2%); key.js's list (fmt.sprintf, sql DB.query/exec/query_row, Rows.scan, json/yaml marshal/unmarshal, logrus
  info/with_field, zap any, pgx Conn.query/exec, redis Client.get/set) all bound.
- Real apps it unblocks (the tester's programs/usego-libs and likes.md "blocked"): a database/sql or pgx CRUD service
  (Query + Scan), a JSON/YAML config loader through Go's decoders, logging through logrus/zap with fields, Sprintf-style
  formatting of Go values, testify-based checks in Go-interop tests.

## Consequences

- One new std type and a few functions in std's `go` module; the binder maps any; adapters convert at the boundary
  (reflect only in `as_list`/`as_map` of non-[]any slices and maps).
- go.Any values are opaque to Xo's type system: errors move to run time at the `as_*` sites (None), which is the honest
  cost of Go's dynamic API; the static parts of a binding are unchanged.
- The output form fixes Scan/Unmarshal and later bug 18's Read without a general `var`-parameter path to Go.

## Alternatives

- **Implicit conversion of any bindable value to go.Any at a call** (no `go.any(...)`): fewer characters at Sprintf and
  Query sites, but a hidden boundary crossing and an ambiguity with handle parameters. Could be added later for
  variadic `...any` only.
- **Map `any` to an Xo sum type** (Int | Str | ... | Handle): closed, matchable, but every Go value outside the sum (a
  struct, a time.Time inside a map) is lost, and it cannot round-trip Go values back.
- **Bind `any` as `Str` via %v / JSON**: lossy, one-way.
- **Keep skipping** (today): 38% of std's and 70% of third party's skips stay.

## Open questions for the owner

1. Explicit `go.any(x)` everywhere, or implicit for `...any` variadics only?
2. Name of the output form: `go.out[T]()` / `.any()` / `.get()`, or `go.ptr[T]()`?
3. Replay of go.Any results: display-only replay (as above) or require `replay rerun` for any-returning functions?
