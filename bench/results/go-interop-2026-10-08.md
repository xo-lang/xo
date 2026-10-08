# Go library interop prototype (2026-10-08)

Experiment for decision 0041 (work queue item 6): call Go packages from Xo
through generated bindings, measure how much of a typical Go API maps
cleanly, and find where hand written wrappers are still needed.

## What was built

- Syntax: `use go "<import path>" [as name]` (parser: `ast.UseDecl.Go`).
  The default local name is the Go package name (`net/url` is `url`,
  `math/rand/v2` is `rand`).
- `xo bind [--dir=D] [--json] <paths...>` (`cmd/xo/bind.go`,
  `internal/gobind`): runs `go list -e -export -deps -json` in a binding
  workspace module (`.xo/gobind/_mod`), loads the export data with
  `go/importer`, walks the package with `go/types`, and writes
  `.xo/gobind/<path>.xo` (bodiless declarations in the `std/*.xo` form) and
  `.xo/gobind/<path>.go` (adapters in package `rt`). Standard library
  only; no new dependency of the compiler.
- `xo check`, `xo build`, `xo run`, and `xo test` bind a `use go` package
  on their own when the cache has no binding, or one written by another
  binder version, or one whose module version differs from the xo.mod pin.
- `std/go.xo`: `go.Error` (an opaque error with `display`, `message`,
  `go_type`), and `std/rt/gointerop.go`: the boundary helpers (copying,
  panic recovery, the task's `context.Context`).
- Effect label `go`: every bound function carries `uses go`. A `pub` function
  that calls Go must declare it (XO0701 otherwise), contract clauses
  cannot call Go (they must be pure).
- Diagnostics XO1101 (package cannot be bound: not found, does not build,
  main package) and XO1102 (the name exists in Go but the binding skipped
  it; the message carries the reason).
- Codegen: a binding module is a std like module whose Go name prefix is
  `Go_<path>_`; calls lower exactly like std calls (`rt.Go_regexp_compile(tk,
  s)`, `re.M_find_string(tk, s)`), and internal/gen adds each adapter to the
  build's `rt/` plus the workspace's `require` lines and `go.sum`.

## Type mapping as implemented

| Go | Xo | At the boundary |
|---|---|---|
| `(T, error)`, `error` | `T ! go.Error`, `() ! go.Error` | error text captured into `go.Error` |
| `(A, B)`, up to 6 results | tuple `(A, B)` | |
| `*T` result of a `New*`/`Must*` function | `T` | nil faults |
| other `*T` result, `*T` field | `T?` | nil becomes `None` |
| struct and array types (`*T` or `T`) | opaque handle `struct{ P *T }` | value `T` is copied in and out |
| interfaces, named maps/slices with methods | opaque handle `struct{ V T }` | |
| exported struct fields | getter `x()` and setter `set_x(v)`; static `T.new()` | |
| string, bool, sized ints, floats, rune | Str, Bool, Int/IntN/UIntN, Float/Float32, Rune | int and uint widen to Int/UInt64 |
| named basic types (`uuid.Version`) | their basic type | methods on them are skipped |
| `[]byte`, `[N]byte` | Bytes | copied; wrong length faults |
| `[]T`, `map[K]V` | `List[T]`, `Map[K, V]` | copied both ways; map entries in key order |
| `...T` | `List[T]` parameter | |
| `func(A) R` parameter | `fn(A) -> R uses fx` with an effect parameter | the Xo closure runs on the calling task |
| `iter.Seq[T]`, `iter.Seq2[K, V]` result | `List[T]`, `List[(K, V)]` | collected |
| leading `context.Context` | not a parameter | `rt.GoContext(tk)`: done when the task is canceled |
| `time.Duration`, `time.Time` | Duration, Time | |
| exported constants | `const NAME: T = <literal>` | |
| exported variables | zero argument function `name()` | read on each call |
| panic | fault at the calling `.xo` line | `defer rt.GoBoundary(...)` in every adapter |

Equality and hashing of handles: identity for pointer handles of structs,
by value for array handles (`uuid.UUID` compares by value), interface
equality for interface handles.

## Coverage of the validation packages

Counted by `xo bind` (the `Coverage:` line at the top of each declaration
file). Functions are the package's exported functions; methods are the
method sets of its exported types; foreign handle methods are methods of
types from other packages that the package's signatures expose (for
example `hash.Hash` from `sha256.New`).

| Package | Functions | Methods | Fields | Constants | Variables | Foreign handle methods |
|---|---|---|---|---|---|---|
| crypto/sha256 | 4/4 | 0/0 | 0/0 | 3/3 | 0/0 | 5/5 (hash.Hash) |
| encoding/hex | 12/12 | 0/1 | 0/0 | 0/0 | 0/1 | 4/4 (io.Writer, io.WriteCloser, io.Reader) |
| regexp | 8/8 | 40/40 | 0/0 | 0/0 | 0/0 | 1/1 (io.RuneReader) |
| strings | 57/57 | 22/22 | 0/0 | 0/0 | 0/0 | 4/4 (io.Writer, unicode.SpecialCase) |
| strconv | 32/34 | 2/2 | 2/3 | 1/1 | 0/2 | 0/0 |
| net/url | 10/10 | 28/30 | 13/14 | 0/0 | 0/0 | 0/0 |
| github.com/google/uuid v1.6.0 | 29/31 | 20/28 | 2/2 | 8/8 | 6/6 | 6/6 |
| **six std packages** | **123/125 (98%)** | **92/95 (97%)** | | | | |
| **all seven** | **152/156 (97%)** | **112/123 (91%)** | | | | |

Skipped, with the reason the binder recorded:

- encoding/hex: `InvalidByteError.Error` (method of a named basic type;
  `InvalidByteError` is a byte), `ErrLength` (sentinel error variable).
- strconv: `FormatComplex`, `ParseComplex` (complex128 has no Xo type),
  `NumError.Err` field (error outside a final result), `ErrRange`,
  `ErrSyntax` (sentinel error variables).
- net/url: `EscapeError.Error`, `InvalidHostError.Error` (methods of named
  string types), `Error.Err` field (error outside a final result).
- uuid: `Must(uuid UUID, err error)` and `IsInvalidLengthError(err error)`
  (error parameters); `Domain.String`, `Version.String`, `Variant.String`,
  `Time.UnixTime` (methods of named basic types); `NullUUID.Scan`
  (`any` parameter); `UUID.Scan`, `UUID.UnmarshalBinary`,
  `UUID.UnmarshalText` (pointer receivers on a value type: through a
  shared handle they would mutate every copy).

Mapped is not the same as pleasant. Of the mapped declarations:

- 5 in net/url return an Option because Go returns a pointer that may be
  nil (`URL.ResolveReference`, `URL.JoinPath`, `URL.User`, `url.User`,
  `url.UserPassword`), although only the `User` field is ever nil in
  practice. One in regexp (`Regexp.Copy`).
- 40 declarations across the seven packages mention a foreign handle. Those
  map but are mostly unreachable: `hex.NewEncoder(w: Writer)` needs an
  `io.Writer` handle, and the only way to get one is from another Go
  function in the same binding. A `strings.Builder` handle is not an
  `io.Writer` handle (gap below).
- 11 take an Xo closure (`strings.Map`, `FieldsFunc`, `IndexFunc`,
  `Regexp.ReplaceAllStringFunc`, ...). These work, including faults raised
  inside the closure, which pass through the Go frames unchanged.

Wider sample (bound with `xo bind`, every adapter compiles with rt, every
declaration checked by `TestGoBindingSignatures` for 20 of them, 2056
declarations): net/http functions 44/45, methods 103/108; os 57/63,
64/68; bytes 60/60, 35/35; math/big 5/5, 139/141; net 45/45, 195/202;
encoding/json 6/9, 20/26; time 9/19, 6/18; log/slog 27/41, 47/67;
database/sql 4/5, 48/84; context 1/12, 2/4; slices 0/40 and maps 0/10
(all generic). The dominant skip reasons there: `any` parameters (slog,
sql, json), generic functions (slices, maps), methods of named basic types
(time.Month, time.Weekday, slog.Level), pointers to non handle types
(`*time.Time` receivers and parameters in package time, since time.Time
maps to the Xo Time value), channels (time.After, context.Done), and functions returning
functions (context.WithCancel).

## Golden programs

`internal/gen/testdata/golden/go_*` run under every golden mode (default,
`--no-mono`, `--arenas`, race):

- `go_sha256`: `sum256` (a `[32]byte` result as Bytes), constants, the
  streaming `hash.Hash` handle (`write`, `sum`, `reset`), handle equality.
- `go_hex`: encode and decode, `go.Error` text and `go_type()`
  (`hex.InvalidByteError`), `hex.dump`.
- `go_regexp`: find, find all, submatches (`List[List[Str]]`), index pairs,
  replace with `$1`, replace with an Xo closure, compile errors, and
  `must_compile("(")`: the Go panic becomes `fault: Go panic in
  regexp.MustCompile: ... at main.xo:30`, exit 70.
- `go_strings`: tuples from `Cut` and `ReadRune`, callbacks, variadic
  `NewReplacer` as a List, `SplitSeq` and `Lines` collected to Lists,
  `strings.Builder.new()` with `write_string`.
- `go_strconv`: errors as `go.Error` (`*strconv.NumError`), a byte
  parameter, `?` in main returning the error with the frame trace.
- `go_url`: getters and setters on `*url.URL`, `url.URL.new()`, the
  nilable `User` field as an Option, `url.Values` (a named map type with
  methods) as a handle, `?` converting `go.Error` to `Error`.
- `go_uuid`: the third party module, pinned in the golden's xo.mod with
  `require go github.com/google/uuid v1.6.0`; parse, `new_sha1` with a
  package variable (`name_space_dns()`), `new_random`, value equality of
  `uuid.UUID` handles. The golden is skipped when the module cannot be
  downloaded.
- `test_go_interop`: Go calls inside `xo test` blocks, a property test, a
  `pub fn ... uses go`, and a Go error failing a test with frames.

## Third party result

`go mod download github.com/google/uuid` works on this host (network or
module cache). `xo bind github.com/google/uuid` runs `go get
github.com/google/uuid@v1.6.0` in the binding workspace, binds it (29/31
functions, 20/28 methods), and builds copy the workspace's `require` lines
and `go.sum` into the build directory's `go.mod`. The program builds with
`CGO_ENABLED=0` like every Xo program.

## Gaps and next experiments

Ranked by how much of a typical API they would unlock.

1. Interface conversion between handles. A handle whose Go type implements
   a foreign interface handle (`strings.Builder` and `io.Writer`) cannot be
   passed where that interface is wanted. Next: emit `fn (h: Builder)
   as_writer() -> Writer` for every pair in one binding where
   `types.Implements` holds, and share foreign handles across bindings by
   giving them one canonical module (`go:io`) so `hex.new_encoder` accepts
   the `io.Writer` from any binding. Touch: `internal/gobind/gen.go`
   `handleFor`, `bindHandle`.
2. Methods of named basic types (time.Month.String, uuid.Version.String,
   slog.Level): bind them as module functions taking the basic type,
   `fn version_string(v: UInt8) -> Str`. Cheap and would recover most of
   the remaining uuid and time skips.
3. `any` and `interface{}` parameters (slog, database/sql, encoding/json):
   accept a closed set (`Str`, `Int`, `Float`, `Bool`, `Bytes`, handles) by
   generating overloads or a `go.Value` enum. Measure on log/slog first,
   where 25 of 41 functions skip for this.
4. Error parameters and sentinel errors (`uuid.Must`, `strconv.ErrRange`):
   bind sentinel errors as `go.Error` values and add `go.Error.is(other)`
   that calls `errors.Is`.
5. The constructor heuristic (`New*`/`Must*` pointer results are non nil)
   should extend to methods that never return nil (`URL.JoinPath`,
   `URL.ResolveReference`). Options: an allowlist in xo.mod, or a static
   check of the Go function body for `return nil` with go/ssa style
   analysis (needs the source, which `go list -json` provides via
   `GoFiles`).
6. Handles are shared references: `u.set_path(...)` on one `url.URL` value
   is seen through every copy, unlike Xo values. A copy on write variant
   for struct handles that have a `Clone`/value form would restore value
   semantics where Go allows it.
7. Callbacks run on the task that called Go. A Go library that calls back
   from its own goroutine (net/http handlers, `time.AfterFunc`) would use
   the caller's task from another goroutine. Next: give each callback
   invocation its own child task of the binding call's task and add a
   golden with `net/http.HandlerFunc`.
8. Channels (`Chan[T]` handles in decision 0041) and generic functions
   (slices, maps) are not bound. Generic functions could be instantiated
   per use site, since Xo knows the concrete types at each call.
9. `xo.mod` does not yet declare what each Go package may do (decision
   0041 "authority"); `uses go` is one coarse effect. `xo vet` reporting Go
   use per module is not implemented.
10. Bindings are per program build only; record and replay of Go calls
    (decision 0041) is not implemented.
