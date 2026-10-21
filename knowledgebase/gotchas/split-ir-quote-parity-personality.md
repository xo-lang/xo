# The LLVM split missed a symbol after a `(` inside a quoted type name

Status: Observed (2026-10-21)

**Symptom.** A cold `xo test --no-gc --backend=llvm` build (empty backend
cache, so the first build of a program) failed in clang with `use of
undefined value '@xo_x_personality'` in one split part: its functions named
the personality routine but the part had no `declare` of it. Warm caches
hid it (TestNoGCTests passed from cached results), so it surfaced only
when a source change gave a program a new cache key.

**Cause.** split.xo's parse_ir scanned a define line for symbols from the
first `(`. A result type such as `%"e.Result[(), Error]"` holds a `(` inside
a quoted name, so the scan began inside the quotes and read every later
quote as the opposite of what it was: with an odd number of quotes left
the `personality ptr @...` text lay inside a "string" and was skipped.

**Fix.** The scan of the define line starts after the function's symbol.
Test: compiler/xo split_test.xo ("a split part that defines a function
with a personality declares the personality").
