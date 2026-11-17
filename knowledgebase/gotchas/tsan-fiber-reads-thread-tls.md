# ThreadSanitizer reported a use c call's TLS read as a race

Status: Observed (2026-11-17, azure-1, std/sqlite race evidence on
linux/arm64; reproduced on linux/amd64 by TestRaceUseC).

Symptom: `xo test --race` of a program whose tasks call C through
`use c` reported "data race ... by thread T11: #0 xo_c_enter" (a read of
size 8), the previous write by the pool thread's
`__tsan_tls_initialization`, "Location is TLS of thread T13". std/sqlite's
race run failed on it; the C library itself was not involved.

Cause: every `use c` call is wrapped in the adapter's xo_c_enter and
xo_c_leave marks (decision 0675), which read the worker record from
`xo_tlocal` and bump its C call sequence word. Under --race a task runs on
a TSan fiber that switches with no synchronization
([tsan-fiber-switch-sync.md](tsan-fiber-switch-sync.md)), so the fiber has
no happens-before with the thread's TLS initialization, and the TLS read
looks racy. Adding an acquire edge at task entry would hide real races
between tasks (that gotcha), so the edge is not the fix. TSan also
instruments atomics in a `no_sanitize("thread")` function, so marking the
functions alone would leave the word's relaxed atomics reported against
the worker record.

Fix: under ThreadSanitizer (`__has_feature(thread_sanitizer)` or
`__SANITIZE_THREAD__`) the adapter defines the marks with
`__attribute__((no_sanitize("thread")))` and plain volatile accesses
(compiler/cli/bind/gen.xo `c_mark_prelude`); other builds keep the relaxed
atomics. Test: internal/llvm TestRaceUseC (testdata/race_usec: 20 rounds
of 32 tasks calling C at once, 3 runs of `xo test --race`).
