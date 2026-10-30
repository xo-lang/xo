# A Go constraint interface is not a type a binding can name

Status: Observed (2026-10-29, Xo Lang 2's Go workflows suite)

## Symptom

`use go "go.uber.org/zap"` fails with "go build failed (a code generator
bug)"; the adapter's error is `cannot use type zap.ObjectMarshalerPtr[T]
outside a type constraint: interface contains type constraints`.

## Cause

gobind's handleFor bound every exported named interface as a handle,
generic ones included since decision 0415. zap's `ObjectMarshalerPtr[T]`
is `interface{ *T; ObjectMarshaler }`: an interface with a type term is a
type set, usable only as a constraint (`types.Interface.IsMethodSet()` is
false), so the adapter's `struct{ P zap.ObjectMarshalerPtr[T] }` does not
compile, and one bad declaration fails the whole package's binding.

## Fix

handleFor and genericBindable skip an interface that is not a method set
("constraint interface X is not a type"); a function constrained by one
stays skipped by typeParams. TestGenericTypes covers a generic one
(`LenPtr[T]`) and a plain union (`Number`). When a binding fails to
build, look for a declaration Go accepts only in a constraint position.
