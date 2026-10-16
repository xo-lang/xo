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
- ThreadSanitizer instruments only functions marked sanitize_thread, a
  mark a C frontend sets and xo's IR never had, so linking xo's IR with
  -fsanitize=thread instrumented nothing of Xo (TestGoldenTasks still
  does that). runRuntimeXo marks the test program's own functions
  (markTSan), with the runtime's fast paths inlined into them: the same
  scope the C drivers' own instrumented code had. Marking the whole
  runtime reports plain accesses to words other threads update
  atomically (obj.xo reserve_slow, tasks.xo scope_close against
  stdio.xo flush_out, pool.xo find_work against the monitor): a data
  race by the C11 and LLVM memory models though not on arm64's
  hardware, left for a later change.
- The first such race in scope was obj.xo's in place append check
  (reserve, rs_push_word, rs_push_words), a plain load of a string's
  count while another thread drops its reference atomically; it loads
  the count with a relaxed atomic now (slot_load: the same ldr on
  arm64).
- rc_threads.c (548 lines) is internal/llvm/testdata/rt/rc_threads:
  shared strings, lists and a 10,000 cell chain captured by eight tasks
  that start together (a barrier), copies on write, the claim (at most
  one task updates in place), appends, a channel queue, literals, and
  the model of random retains, releases, claims and marks on raw
  objects (rt_probe). Not kept: the chain cells' own counts after the
  share walk (rt.obj_addr of an enum is XO1201 on the native backends;
  the leak check and the cells' values are), and the C's hand written
  share walkers, which generated code replaces.
- alloc_threads.c (328 lines) is internal/llvm/testdata/rt/alloc_threads:
  solo (before the program's first task), threads (eight tasks
  together, mailboxes crossing threads), waves (frees on other workers,
  checked, with the chunk bound). Every C check kept.
- Batch 2 (2026-10-16), each verified on darwin/arm64 and, in Docker,
  linux/arm64, both backends, one worker, three, and eight:
  - evloop.c and rawpoll.c are testdata/rt/evloop: timers (2000
    sleepers; the workers alive stay at most the CPUs plus one, rt_probe
    "workers" and "cpus"), an 8 MB echo, wakeups, registrations
    (rawpoll's descriptors registered while the poller waits), and
    inherited (rt_probe "inherited": linux.xo's pipe2 without
    close-on-exec, a forked child that sleeps, pd_open and pd_free, a
    write; Linux only, first run on Linux by this rewrite). Not kept:
    rawpoll's ThreadSanitizer edge between a registration and the
    poller, which needs poll.xo instrumented (the queued TSan-clean
    runtime).
  - fault_shared.c, fault_shared.ll and fault_resume.c are
    testdata/rt/fault_shared: rt_probe "qsort_fault" calls the C
    library's qsort (sys_c.xo) with cabi.xo fault_raise_msg as the
    comparator, so the fault crosses libc's frames; task.isolate catches
    it, or (XO_FAULT_UNISOLATED) it is reported past the boundary with
    exit status 70. Not kept: fault_resume's case of a C frame's cleanup
    resuming the unwind (x86_64 and Windows, where libgcc or the system
    unwinds); an Xo program has no C frame with a cleanup.
  - realtime.c is testdata/rt/realtime, every check kept: the frees
    deferred inside a section (std/internal rt_queued, real-time), a
    real-time call inside a section, the queue's overflow at 256, a
    counted allocation (rt_allocs) and its XO-F007 caught by
    task.isolate, and sections on several tasks at once. Values are
    released inside a section by stores into a List nothing else holds:
    parameters are borrowed, so a passed value is released after the
    section. For rt_queued and rt_allocs to be real-time, the pure loads
    rt.mem_load, rt.slot_load, rt.static_word, and rt.tlocal_get are
    declared `uses realtime` in std/native/rt.xo (the lead's decision:
    a declaration, not a new intrinsic; rt is std internal, so core 6.5's
    rules for programs do not change), and obj.xo rt_heap is heap()
    without its slow path. No store or allocating intrinsic is marked.
- C kept on purpose: std/llrt/testdata/win_consts.c (static asserts of
  the Windows SDK's constants and struct layouts the runtime hard codes:
  only a compile against the real headers checks them),
  internal/llvm/testdata/errno_probe.c, the `use c` test data
  (internal/cbind, internal/selfhost's tiny.c), and
  examples/realtime-control.
- Later: crypto_diff.c and crypto_ref.c (dropped if the Go diff tests
  cover them), then the exports and xcalls.h and xrec.h only they used.

## Alternatives

- Keep the C: it is the last C in the runtime's tests and pins exports
  generated code no longer calls.
- Xo unit tests inside std (`_test.xo` of std/native/llrt): `xo test`
  has no way to run a std module's tests, and the stress tests need
  ThreadSanitizer builds anyway.
- New rt intrinsics for the internals: an owner decision each; the
  existing functions were enough.
