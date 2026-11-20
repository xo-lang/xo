# A --race report of a free against an earlier read: the runtime's release was not told to ThreadSanitizer

Status: Observed (2026-11-19 and 2026-11-20, task #437)

## Symptom

internal/llvm TestRaceOrphanScope failed in 9 of 34 llvmpre gate logs on
the 20 CPU hosts with one ThreadSanitizer report: a write by the main
thread in `free`, called from `obj.h_free_n` (std/native/llrt/obj.xo),
against a previous read of the same 8 bytes by the test's fiber in
`http.ClientStream.read`. Outside a gate it never failed: 0 of about
1,440 `xo test --race` runs of testdata/race_orphan and 0 of 80 `go
test` counts on host 3 under load 40 with the gate's environment
(notes/xo-lang-1/p437, 2026-11-20).

## Cause

A handle's first word is its count. Generated code reads it before its
atomic decrement, and in a --race build its drop helper tells
ThreadSanitizer the order (`obj.dec_shared_race`: a release before the
decrement, an acquire when it was the last). The runtime is not
instrumented in a --race build (decision 0273 mode 2), and its own
releases of a handle (`tasks.h_release`, `stop.xo` `h_release`,
`h2srv.xo` `gone_close`) called `rc.dec` and `obj.h_free_n` with no
such calls. When the runtime dropped the last reference on another
thread, ThreadSanitizer saw the free with nothing ordering it after the
other thread's read of the count word. The program was correct; the
report was a missing happens-before.

## Fix

`obj.h_dec` (release, then `rc.dec`) for the runtime's releases, and
`sanhook.acquire` at the start of `obj.h_free_n`. A new place where the
runtime drops a handle's reference calls `obj.h_dec`, never `rc.dec`.
internal/llvm TestRaceRuntimeHandleRelease reads the --race IR for
both; it fails on the source before the fix. The race itself has no
test that fails before: it did not reproduce outside the gates.
