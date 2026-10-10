# 0112: Values that contain handles are not comparable with `==`

Status: Accepted (2026-10-09, by the owner; not yet implemented)

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
