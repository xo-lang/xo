# ThreadSanitizer on xo's LLVM IR instruments nothing without a mark

Status: Observed (2026-10-16, decision 0241)

## Symptom

A program built from xo's LLVM IR with `clang -fsanitize=thread` links
the ThreadSanitizer runtime and runs, but never reports a race in Xo
code, and runs at nearly native speed.

## Cause

clang's ThreadSanitizer pass instruments only functions with the
`sanitize_thread` attribute, which the C frontend adds under
-fsanitize=thread; IR given to clang keeps the attributes it has, and
xo's has none. So the C stress tests' own C was instrumented, and an Xo
program's code never was (TestGoldenTasks builds its goldens that way
today: the runtime's interceptors and annotations run, the Xo code is
not checked).

## Fix

Mark the functions to check: internal/llvm runRuntimeXo's markTSan adds
`sanitize_thread` to the definitions of the test program's own
functions (before a `personality`, where function attributes go).
Marking every function, the runtime's too, reports plain accesses to
words other threads update atomically in the scheduler, stdio, and the
heap (decision 0241's list), which have to be fixed before the whole
runtime can be checked.
