# TestRuntimeIntrinsicsLowered reads any quoted runtime name as a lowering

Status: Observed (2026-10-28, 0a58e8e53); fixed in the test the same day

Symptom: internal/repotest TestRuntimeIntrinsicsLowered fails with
"intrinsic_gaps.txt lists `arm64 cast` but the arm64 backend lowers rt.cast
now: remove the line", after a change that lowers nothing new.

Cause: the test decides that a backend lowers rt.N when its sources
contain the quoted text "rt.N" or "N" anywhere. compiler/arm64/stacksmall.xo
(decision 0395) named rt.cast, and a list of read-only runtime calls, as
quoted literals to recognize them, which the test took as lowerings.
Removing the gap line would have hidden a real gap.

Fix: the test now reads only the backends' lowering functions (lowerFns
in internal/repotest/intrinsics_test.go: rt_call, inline_rt, inline_raw,
atomic, LLVM's xo_export, wasm's call and its helpers) and fails if one of
them is renamed, so a pass may name runtime calls as plain literals again
(stacksmall.xo does). The first fix forward, a940023ce, hid the names from
the test instead (a word list in stacksmall.xo); the lead asked for the
test to read where lowering happens. A new lowering function: add it to
lowerFns.
