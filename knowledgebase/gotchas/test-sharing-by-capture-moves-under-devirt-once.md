# A test that shares a value only through a closure capture stops sharing it under devirt_once

Status: Observed (2026-11-17, xo-lang-1, task #268)

## Symptom

TestRuntimeRealtime's `allocs` case failed on main from 59824eb7c (decision 0697, devirt_once):
"0 allocations counted in the section, want 1" and "a section that allocated did not fault".

## Cause

The test made a list shared by capturing it in a closure (`task.isolate(fn() { var keep = xs; ... })`)
and expected the store into it to copy. The caller never read `xs` again, so the capture was its last use.
Once the closure made and called once became a direct call (0697), `xs` was moved in, not shared, and the
store wrote in place with no allocation. Correct under value semantics; the test's premise was wrong.

## Fix

Make the sharing real: read the value after the code under test (the test now checks `xs[0] == 1` after the
section). Any test that needs a shared (reference count above 1) value should keep a live use of it after
the operation, not rely on a closure capture or a missed last-use move.
