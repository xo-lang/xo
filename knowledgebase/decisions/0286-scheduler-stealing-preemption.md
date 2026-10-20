# 0286: Scheduler: work stealing, back-edge preemption, and the root's inline task

Status: Proposed (owner decision 2026-10-20, through the lead; Xo Lang 2).
Stub claiming the number; the baseline measurement and the design come
first.

## Context

Xo Lang 3's A/B against Go (2026-10-20: the bench CRUD server plus an
86 ms CPU handler, 8 CPU clients, 12 cores) puts Xo's dial to response
p99 at 130 to 147 ms against Go's 37 to 46 ms: a request queued behind a
running long handler waits for it even with cores free. Separately (Xo
Publish, gotchas/root-inline-task-waits-for-helped-work.md), a task the
root thread runs inline helps with other work in park_wait while it is
parked and cannot migrate, so its wakes wait behind a long CPU task (17
to 19 of about 20 timer ticks late with await, 0 with a channel wait).
Moving serve() off main made the HTTP case worse, so the root inline fix
alone does not help HTTP.

## Decision (owner, 2026-10-20; design to be written)

1. Work stealing of tasks queued behind a busy worker.
2. Preemption at loop back-edges: a cheap poll of a per-thread flag a
   timer sets (as Go's), kept out of tight loops (hoisted, or only on
   loops without calls); a no-op on wasm (one thread).
3. The root inline task's wake no longer waits behind work the root
   helps with.

Stealing first, then preemption, each measured with Xo Lang 3's harness
against Go (8 and 16 CPU loops, p50 and p99, a quiet machine), and
preemption's cost on the compute benches (portable, checked, json,
crypto). Gates: the fixed point, the arm64 and amd64 goldens, wasm.

## Predictions

Each is rechecked after stealing and after preemption, and marked
confirmed, partial, or not confirmed once measured.

Very likely:

1. HTTP p99 with free cores: Xo 130 to 147 ms against Go 37 to 46 ms
   today (Xo Lang 3's harness, 8 and 16 CPU loops).
2. Timers, deadlines, and cancellation late under CPU load (Lifeline
   deadlines, `xo test --timeout`, keep-alive and idle timers,
   Proc.set_grace); the ticker repro of the root inline gotcha.
3. Head-of-line blocking in servers, a fast request behind a slow one on
   one worker (HTTP, xo lsp without its channel workaround, xo serve, the
   compile server of decision 0283).

Plausible:

4. Timing-sensitive flakes under load (clock_timers, TestRuntimeEventLoop,
   TestNoGCTimeout's slow run).
5. xo's own parallel emit: scope_join and run_claimed may serialize parts
   on the root (an xo trace of a cold self build before and after; emit
   wall time).
6. The rare dev-goldens build timeouts (the hang sampler's output).

Not expected to change: resets under saturation (the kernel's backlog of
128, the same for Go), the TestH2Diff 431 flake, throughput benches.

| prediction | check | after stealing | after preemption |
|---|---|---|---|
| 1 | harness p50/p99, 8 and 16 CPU loops | | |
| 2 | ticker repro (late ticks of 20), timer tests | | |
| 3 | fast request latency behind a slow handler | | |
| 4 | the flaky tests, 50 runs each under load | | |
| 5 | emit wall time and trace of a cold self build | | |
| 6 | dev-goldens timeouts over a day of runs | | |
