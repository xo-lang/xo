# A new prelude struct is XO1201 natively until the native prelude declares it

Status: Observed (2026-10-30)

## Symptom

A change adds a plain data struct to std/prelude.xo (`ProcUsage`,
`DiskUsage`, decision 0426) and returns it from a native prelude
function. `go run ./scripts/xobuild` stops before building anything:

```
xotool: no seed built xo ...
  the newest earlier build .../xo: its smoke build: exit status 1
  note: support: planned; build with --backend=go to use it until then
```

The smoke build (a hello world built by the seed against the change's
std) hides the error itself; run it by hand to see it:

```
error XO1201: not available on the native backends (arm64, amd64, LLVM) yet: the type `ProcUsage` (support is planned)
```

## Cause

compiler/ssa build.xo treats a prelude struct as an opaque handle
unless it is in a short list (`DurationErr`, `FileInfo`, `ProcOutput`,
`Target`) or the native prelude (std/native/prelude.xo) declares a
struct of the same name, its implementation type (`impl_type`). Adding
the name to the list would work only for a compiler that has the list:
the seed that builds xo does not, so the std it builds rejects the type.

## Fix

Declare the struct again, with the same fields, in std/native/prelude.xo
(a comment naming this gotcha). The seed already looks implementation
types up by name, so no seed bump is needed; std/native modules and
std/native/llrt that build the type with a struct literal still name the
prelude's declaration, which maps to the twin.

To see a smoke build's error: build a hello world with the seed in
`~/Library/Caches/xo/checkout/<key>/xo build --backend=arm64
--std=WORKTREE/std -o smoke DIR` (through heavy.sh).
