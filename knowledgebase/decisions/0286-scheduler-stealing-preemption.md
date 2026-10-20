# 0286: Scheduler: work stealing, back-edge preemption, and the root's inline task

Status: Proposed (owner decision 2026-10-20, through the lead; Xo Lang 2).
Step 0 (the root's hand off) implemented 2026-10-20; main on a fiber next,
then stealing and preemption as measured.

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

## Step 0: what the trace showed, and the root's hand off (2026-10-20)

`xo trace` of the cpuburst server (8 /cpu loops, a 200 x 3 burst, Mac at
load 20 to 33) put every slow wake (22 of 182 over 10 ms, 67 to 76 ms)
on the accept loop running inline on the root: the poller woke it, but it
ran only after the root finished an 86 ms handler it had taken in
park_wait while the loop was parked, with 9 or 10 of 11 workers idle. In
mode B (serve() in a spawned task) main's scope join had claimed the serve
task and run it inline on the root, so the same happened. Stealing was not
what was missing; the root's help was.

- v1, the root leaves all queued work to the workers when one is idle or
  can start, and sleeps: HTTP A dial to response p99 70.3 to 6.3 ms,
  failed connections 225 of 600 to 0, wake to run p99 75.9 to 4.2 ms;
  the ticker repro 22 to 29 late ticks of about 25 to 0. But ping pong
  878 to 1084 ms against 24 to 31 (every hop a thread's wake). Rejected.
- v2 (pushed as step 0): the same, but the root still runs its own next
  item, the fiber its parked task just woke (a channel's partner, which
  hands back in microseconds): ping pong 26 to 33 ms (main 24 to 31),
  spawn and fanout within the noise of main, HTTP A p99 12.7 ms against
  70, internal/llvm acceptbusy 0 to 2 ms against about 800. The ticker
  repro (rootpark) still fails at times (164 ms once, main 394 ms): a
  dispatcher's send puts the computing worker in the root's next slot.

The root cannot tell a hand back of microseconds from a long computation
before running it; what is missing is that main's body cannot move to
another thread. So the next step (approved by the lead, 2026-10-20,
within the owner's runtime fix) is main's body on a fiber, as Go runs
main as a goroutine: the root thread runs fibers like a worker, main's
first; a join's inline task then runs on main's fiber too; a woken main
can be stolen by an idle worker while the root computes. Gates: the fixed
point, every golden on arm64, amd64 (the Docker sample) and LLVM, the
scheduler tests, the fault and traceback goldens (the unwinder's stack
bounds), ThreadSanitizer's same thread tests, and the Windows kit's
no-CRT stack checks; ping pong and spawn in each report.

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

| prediction | check | after step 0 (v2) | after main on a fiber | after stealing | after preemption |
|---|---|---|---|---|---|
| 1 | harness p50/p99, 8 and 16 CPU loops | partial (Mac trace, load 33: p99 70 to 12.7 ms; CI pending) | | | |
| 2 | ticker repro (late ticks of 20), timer tests | partial (rootpark 394 to 0 to 164 ms, flaky) | | | |
| 3 | fast request latency behind a slow handler | partial (acceptbusy about 800 to 0 to 2 ms) | | | |
| 4 | the flaky tests, 50 runs each under load | | | | |
| 5 | emit wall time and trace of a cold self build | | | | |
| 6 | dev-goldens timeouts over a day of runs | | | | |
