# Prelude handle types are opaque outside the native prelude

Status: Observed (2026-10-15)

## Symptom

A native std module (std/native/testing.xo) that builds or reads a
prelude handle type fails to check:

```
error XO0605: `File` is an opaque handle and cannot be built with a struct literal; use its constructor function
error XO0601: `File` has no field or method `mem`
error XO0401: expected `Bool`, found `fn(Bytes) ! FsErr uses fs.write`
```

The last one is `f.write` read as the method `File.write`, not the
native struct's `write` field.

## Cause

std/prelude.xo declares `File`, `FileLock` (and the other handles) as
empty structs, which the checker treats as opaque handles. Only the
native prelude module (std/native/prelude.xo) sees the native struct's
fields; every other module, native ones included, sees the empty
declaration. `rt.cast` does not help: it converts only a std type and
its own implementation type.

## Fix

Give the native prelude small `llrt_<name>` functions that build or
read the handle, declare `fn <name>(...)` in std/native/rt.xo, and have
selfhost/ssa call.xo `intrinsic` lower `rt.<name>` to the prelude's
function with `llrt_or_none` (as `rt.fs_canceled` and `rt.mem_file`
do). The new rt functions are not used by the compiler's own code, so no
seed bump is needed.

Related: `==` on a value that holds a handle (`Result[File, FsErr]`) is
XO1201 natively; compare the error alone (`err_of` in
cmd/xo/testdata/nogc/same/fakes_test.xo).
