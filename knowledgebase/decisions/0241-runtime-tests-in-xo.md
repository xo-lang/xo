# 0241: The runtime's C stress tests rewritten in Xo

Status: Accepted (2026-10-16; the lead's assignment under the owner's
"get rid of C" priority; the std/internal probe approved by the lead on
the condition that it uses existing intrinsics and exports only)

## Context

After the zero-C work the native runtime has no C, but its stress tests
did: std/llrt/testdata held 3,578 lines of C (13 files) that played the
part of generated code against the runtime's C ABI exports (cabi.xo),
compiled with ThreadSanitizer by internal/llvm's tests. They keep 20
exports alive that nothing else calls (the 0.17.0 cleanup kept them),
and they test the runtime through an interface the language never uses.

## Decision

Each C stress test becomes an Xo program under
internal/llvm/testdata/rt/NAME (module path under github.com/xo-lang/xo),
built to LLVM IR, linked with ThreadSanitizer, and run with one worker,
three, and eight and the leak check on (`runRuntimeXo`), then built
optimized by xo and run once with more work. Each prints `TEST: ok` per
test as the C did, and the run must end with `xo leakcheck: 0 live`.

- The language replaces what the C did by hand where the language can:
  scopes, spawn, await, cancel, background tasks, Chan, Mutex, Atomic,
  shared values captured by tasks.
- What the language does not show comes from a probe in std/internal
  (visible only to std and the toolchain's modules, decision 0214):
  `internal.rt_probe(op, a, b, c, d) -> Int` over the runtime's existing
  functions (obj.xo's blocks, heapos.xo's counts and chunks, rc.xo's
  counts, a mutex's queues, a scope's list) and `internal.obj_word(x)`
  (rt.obj_addr). No new intrinsic. The Go backend faults on both.
- A task waiting on its child runs the child on its own thread, so a test
  that needs another thread (frees crossing threads) spins on an Atomic
  until the child is done: another worker must take it; "thread"
  (rt.tlocal_addr) checks that it did.
- Tasks replace pthreads. With one worker nothing crosses threads; the
  three and eight worker runs carry the cross thread coverage.
- The C file, its harness code, and the exports only it called go in the
  same change; an assertion the Xo version cannot keep is listed in the
  change.

## Consequences

- tasks.c (561 lines) is internal/llvm/testdata/rt/tasks: every C check
  kept but two that observe the body's unwinding from inside (the
  receive's empty value while unwinding, and t_unwinding after the
  scope's close); the language unwinds the body, so the Xo test checks
  the scope's error and `task.is_canceled()` instead. prune's bound is
  now that the list never held every task started (it stays at one
  batch at rest): tasks.c's three quarters of them is timing dependent
  and failed 1 in 13 runs at a load of 30. magazines' frees run on
  another worker (the allocating task spins until they are done) and
  check that they did. The ThreadSanitizer runs take about 1 s warm
  (2026-10-16).
- A timing dependent check is not carried over as it was: every session
  runs at a load near 30, where it would flake for everyone.
- Later batches: rc_threads.c, alloc_threads.c; evloop.c, rawpoll.c,
  fault_resume.c, fault_shared.c, realtime.c, win_consts.c; crypto_diff.c
  and crypto_ref.c (dropped if the Go diff tests cover them).

## Alternatives

- Keep the C: it is the last C in the runtime's tests and pins exports
  generated code no longer calls.
- Xo unit tests inside std (`_test.xo` of std/native/llrt): `xo test`
  has no way to run a std module's tests, and the stress tests need
  ThreadSanitizer builds anyway.
- New rt intrinsics for the internals: an owner decision each; the
  existing functions were enough.
