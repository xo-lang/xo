# TestRuntimeEventLoop's dial failed once under load

Status: Obsolete (2026-10-16): evloop.c is the Xo program internal/llvm/testdata/rt/evloop (decision 0241), whose dial faults with its error. Was observed once (2026-10-16, darwin/arm64), not reproduced.

## Symptom

In one `go test -short` run over the repo (09:00 on 2026-10-16, while
other sessions ran heavy tests and benchmarks on the same Mac),
internal/llvm TestRuntimeEventLoop failed:

    ../../std/llrt/testdata/evloop.c:236: check failed: t_net_status() == 0
    ThreadSanitizer:DEADLYSIGNAL
    ==18748==ERROR: ThreadSanitizer: SEGV on unknown address 0x000000000000 (pc 0x000103446d34 bp 0x00016d1e64c0 sp 0x00016d1e6480 T150433036)
    ==18748==The signal is caused by a READ memory access.
    ==18748==Hint: address points to the zero page.

Line 236 is test_echo's dial of the test's own listener on 127.0.0.1.
The same day Xo lang 4 saw TestRuntimeTasks fail once in -short
(threads_test.go:283: prog: exit status 1), then pass.

## Cause

Two parts:

- The SEGV is the test's: CHECK only counts a failure, so test_echo went
  on with the zero handle t_net_dial returns on an error and read
  through it. Fixed: a failed dial now prints its status and message
  (xcalls.h t_net_msg) and exits 1.
- The dial's failure itself is unknown (the old test printed neither
  its status nor its message). Not reproduced: 0 failures in 50 runs
  alone, 60 runs beside three full -short runs (TIME_WAIT sockets peaked
  at 175, so not ephemeral port exhaustion), and 100 runs of it and
  TestRuntimeTasks with 16 CPU hogs on 12 cores.

## If it happens again

The new output names the dial's error (status 2 refused, 4 an Io error
with errno text, 5 canceled; netconn.xo dial_timeout). Keep the whole
-short log and note what else ran on the machine. A refused dial to a
listener that exists would point at the listener's backlog or a stale
poll descriptor (pd reuse after pd_free); an Io error at the system.
