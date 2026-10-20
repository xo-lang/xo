# A struct holding a handle, converted to an interface, failed to build natively

Status: Observed (2026-10-20, a database driver struct, decision 0278)

## Symptom

A program converting a value whose type holds a handle (a `Mutex`, an
`Atomic`, a `Chan`, a `Conn`) to an interface failed natively with
`xo: arm64 backend: equality of Mutex[List[Str]]` (LLVM: `the SSA
lowering does not compare ...`). Every std/sql driver struct holds such a
handle. The Go backend built it, and `==` on two such interface values
compared them with `reflect.DeepEqual`.

## Cause

An interface value carries an equality function (compiler/ssa iface.xo
`eq_thunk`) for `==` on interface values. For a dynamic type holding a
function it returns false; for one holding a handle it tried to compare
the handle, which the emitters cannot do (handles are not comparable,
core 2.9).

## Fix

The equality of an interface value holding a handle is false, as for a
function (`eq_thunk`, and std/rt `EqAny` with `holdsHandle` on the Go
backend), so the backends agree and no two drivers compare equal. core
2.9 says so; golden `iface_handle_fields`.
