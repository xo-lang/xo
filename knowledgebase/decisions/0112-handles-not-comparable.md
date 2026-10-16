# 0112: Values that contain handles are not comparable with `==`

Status: Accepted (2026-10-09, by the owner). Implemented 2026-10-09:
`XO0404` in internal/check (`comparable.go`), core 2.9.

## Context

Handles (`Mutex`, `Atomic`, channels, task handles, file, listener and
connection handles, and similar resources) are references to a runtime
object. The Go backend compiles `==` on a struct that contains one
(comparing the Go values), while the native backends report XO1201.
Equal handles would mean identity, which conflicts with the value
semantics of everything around them and is rarely what a program means.

## Decision

A type is comparable only if all its fields and payloads are; handle
types are not comparable. `==` and `!=` on a value whose type contains a
handle (directly, in a field, an enum payload, a tuple, or a collection
element) is a compile error on every backend, with a note pointing at
comparing an identifying field instead. Such types also cannot be `Map`
keys or `Set` elements, and `derive Eq`/`Hash`/`Ord` on them is
rejected. Automatic `Debug` is unaffected.

## Consequences

- One meaning on every backend; native drops its XO1201 for this case.
- Programs that compared such structs must compare fields; `xo fix` can
  rewrite a struct comparison into a field comparison only when one field
  is obviously the identity (none is guessed otherwise).

## Alternatives

- Identity comparison of handles on every backend: legal but surprising
  next to value equality of the other fields.

## Implementation notes (2026-10-09)

- Code: the existing `XO0404` ("type does not implement a required
  interface or constraint"), which `==` on a function value already
  reported as "does not implement `Eq`"; no new code. Each report
  carries a note naming the path to the handle (``field `inbox` holds
  the handle `Chan[Str]` ``) and one suggesting an identifying field.
- Handle types (internal/check `handleTypes`): prelude `Chan`, `Mutex`,
  `Atomic`, `Scope`, `Task`, `Listener`, `Conn`; std/task `Group`,
  `Semaphore`, `Once`; std/http `Router`, `Server`, `Client`,
  `BodyWriter`, `TlsCert`; std/sql `Db`, `Tx`; std/testing `FakeNet`,
  `FakeClock`, `MemFs`, `FakeEnv`, `FakeStdio`, `FakeProc`; and opaque
  objects of the native implementation modules. Values such as
  `sql.Row`, `time.Zone`, `http.Key[T]`, `http.Request`, and capability
  interfaces are not handles.
- `use go` opaque values are excluded: they exist only on the Go backend,
  so there is no cross backend difference, and some are Go values that
  compare by value on purpose (`uuid.UUID`, a [16]byte; the go_uuid
  golden compares them).
- `Eq` and `Hash` (`isEqType`) now look into struct fields, enum
  payloads, and newtype targets, not only type arguments, so a struct
  with a function field is not `Eq` either.
- Map and Set keys are checked on type annotations (signatures, fields,
  `let` types) and on the type of every expression after the bodies are
  checked (literals, `to_set`, generic results), once per type and file.
- `derive Ord` keeps its own field check (handles are not `Ord`), so
  only `derive Eq` and `Hash` go through the new check.
- The SSA builder keeps its guard for `==` and hashing of handles, now
  reached only by std types whose native implementation holds one
  (`http.Request` with typed values: `==` on requests is still XO1201
  natively, planned).

## Native `http.Request` equality (2026-10-10)

`http.Request` holds no handle (its fields are strings, a `Url`, a
header map, and bytes), so it stays comparable on every backend; the Go
backend's acceptance was right and the internal/gen golden `http_scoped`
keeps `with == plain`. The native implementation's hidden route
parameters and typed values (an `rt.Any` list) take no part in `==`
(std.md 4, decision 0055), so internal/ssa (`compareValues`,
`isNativeRequest`) compares the six spec fields of
`native.http.Request` field by field, as std/rt's generated `XoEq`. The
XO1201 guard stays for any other std type whose native implementation
holds a function or handle. Test: internal/llvm golden `request_eq`
(also under `-short`).

## Addendum (2026-10-15)

The handle types added after this decision were missing from the
checker's list (selfhost/check infer.xo `is_handle`), so `==` on a
`Result[File, FsErr]` passed the check, compared by identity on the Go
backend, and stopped natively at XO1201. They are handles in the sense
above (std.md calls each "a handle"), so the rule applies unchanged and
they join the list: prelude `File` and `FileLock` (decision 0163) and
`Child` (std.md 3.8), and std/internal `SharedLock` (decision 0214).
`regexp.Regex` stays comparable (lead, 2026-10-15): it is an immutable
compiled value like `time.Zone`, and std.md and std/regexp.xo now call
it that instead of a handle, so core 2.9's list is the one source. Test:
selfhost/check check_test.xo ("File, FileLock, Child, and values holding
them are not comparable").
