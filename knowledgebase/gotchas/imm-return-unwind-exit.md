# An always-faulting function's zero result made its callers skip releases

Status: Observed and fixed (2026-10-30)

## Symptom

internal/arm64 TestStdStoreBuilds (`xo dev stdcache`, XO_INLINE_STRICT=1) failed after
82a509844 added `Request.src_close`: `the inlined arm64 module does not verify (74 errors; the
first: http.ReverseProxy.serve: b100: v392.0 is held 1 times at ret (leak or imbalance))`. The
module without inlining verified, and every golden passed.

## Cause

The std store is built without concurrency (XO_STD_CONC=0), where Mutex.with_read lowers to
a fault, so Request.with_body_reader always faults: its only `return` is the unwind exit's,
which returns a zero Request. compiler/ssa rc.xo imm_return_of takes a function's one returned
struct literal and marks the fields made of immortal values (Str and Bytes constants,
rt.rl_empty), and the callers' reference counting leaves those fields of the call's result
unreleased. Inlining later dropped the raise's landing, so with_body_reader had no return left:
the verifier, reading the inlined module, found no immortal fields and counted the 14 fields
serve never released. Each side was consistent with what it saw; the facts changed under it.

2d51a8403 hid it for Request (eight fields or more: the exit returns a zero_val call, not a
literal); a smaller struct still met it.

## Fix

imm_return_of gives no immortal fields when its one return is in the unwind exit: a zero result
there says nothing a caller may keep. Only functions that always fault change.
Test: compiler/ssa immret_test.xo.

## How to find the next one

The verifier names the value and its fields (`v392.0`); compare the fields left held with what
a zero of the type makes immortal, and check the callee for a single return. The tools for the
std store build: XO_INLINE_DENY and a debug print; not XO_PASSES or XO_DUMP_FN (see
std-store-debug-env.md).
