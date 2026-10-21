# 0286: Scheduler: work stealing, back-edge preemption, and the root's inline task

Status: Proposed (owner decision 2026-10-20, through the lead; Xo Lang 2).
Step 0 (the root's hand off) implemented 2026-10-20; step 1 (main on a
fiber) implemented 2026-10-20, accepted by the owner with its ping pong
cost; stealing and preemption next, as measured.

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

## Step 1: main's body on a fiber (design, 2026-10-20)

- Main's thread (entry.xo main, thread_run) becomes the root scheduler:
  a small loop (pool.xo, beside worker_main) that runs main's fiber with
  fiber_run, and when main parks runs queued work or sleeps until main
  is woken or work arrives. Main's fiber is reserved, never committed,
  at main's present stack size, 1 GiB (MAIN_STACK; the worker threads'
  and the task fibers' 64 MiB is not main's: a 64 MiB main would turn
  recursion that works today into a crash), outside the fiber caches,
  with a guard page; on Windows MEM_RESERVE and guard page growth, as the
  task fibers. Checked under Docker --memory=256m and `ulimit -v`, since
  some hosts count reservations.
- A wake of main is a fiber's wake (fiber_wake, then a queue): a worker
  or the root resumes it, so a parked main moves to an idle worker while
  the root computes. fiber_main runs main's entry for main's fiber (a
  marker where a task fiber has its task).
- A program that never spawns stays single threaded (decision 0077): the
  pool's records and inject queue are made for main's fiber, but worker
  threads and the heap's threading (heap_threads) wait for the first
  spawn, as today.
- Deadlock detection and the test world's kick: the root asleep with
  main parked and counted counts as a blocked root, as the root's futex
  sleep does today.
- The program's end stays on the C thread after main's thread returns;
  main's fault boundary (C_BOUNDARY) and root node are set on main's
  fiber.
- rootpark in TestRootParkHandOff then passes, and step 0's root_next
  becomes the root running main's partner on the same thread.
- XO_MAIN_FIBER=0 keeps main on its thread, as before: a kill switch for
  one seed cycle, removed after 0.24.0 with a note here (the lead,
  2026-10-20: two scheduler paths double the test surface).
- Checked (2026-10-20): every golden on arm64, LLVM and Go; the Wine
  subset with the amd64 backend (31 of 31); the Docker amd64 sample (42
  of 42); the scheduler, ThreadSanitizer, fault and traceback, heap guard
  and debug information tests. Bring-up found and fixed a lock order
  inversion (gotchas/scheduler-lock-order.md), a race on main's wait word
  (TSan's runtime mode), and the root's worker record 8 bytes short
  (poolsetup.xo's own W_SIZE, XO_HEAP_GUARD).
- Found in bring-up (2026-10-20): main's thread then needs only the
  root's loop (8 MiB), and main's 1 GiB is reserved in entry.xo main,
  where the thread's 1 GiB was, before main's thread starts. Reserved
  later (in the root's start), the mapping failed under `ulimit -v 1.5
  GiB` where the thread's had not, and main fell back to the root's small
  thread (a segmentation fault at 1 M deep); reserved early, the fiber
  and the thread behave the same: 3 M deep recursion at 4 and 1.5 GiB, a
  clean `fault: out of memory` at 1.4 GiB, and a program in a 256 MiB
  container runs. The root counts as alive and idle while it loops (the
  deadlock check's counts) but not against the worker threads' limit
  (DLK_LOOPS): counted against it, XO_THREADS=1 had no worker at all.

Measured (2026-10-20, single small programs, release arm64, 20
interleaved runs, Mac at load 34 to 44): spawn unchanged (69.2 against
69.6 ms; at XO_THREADS=1 20.6 against 19.5); ping pong 27.7 to 33.9 ms
(+22%, the same at XO_THREADS=1): a parked main helped from its own
frame (two stack switches a round trip with its partner) and now parks
into the root's loop (four), about 60 ns a hop, the price of a main that
can move (Go's main goroutine pays it too). Before it lands the root's
loop is trimmed of what it does that a worker's does not need (the
trim, a parked main counted blocked only when its root goes idle,
gained nothing measurable: the switches are the cost).

On a quiet Mac (load 6 to 7, bench/perf/conc, rounds interleaved,
medians of 5, ms): ping pong main 24.2, main on a fiber 30.6 to 31.5
(+26 to 30%, the same at XO_THREADS=1); spawn 246.7 against 238.1 to
247.1; fan out 94.2 against 90.8 to 92.5. Accepted by the owner
(2026-10-20): the ping pong microbench's cost for a main that moves,
spawn and fan out unchanged, the dispatcher stall (rootpark 164 ms) gone.
In 0.24.0.

## Step 2: stealing (design, 2026-10-20)

After step 0, CI shows queued work reaching free cores (prediction 1).
Stealing exists (pool.xo steal_round: a random victim's ring, half of it;
its next slot only on the last round and after NEXT_GRACE_NS, 50 us), so
step 2 starts with a measurement, not a change: xo trace of the cpuburst
server at 16 and 24 /cpu loops on the 16 vCPU runner (all workers busy,
and oversubscribed), and of a fan out whose tasks spawn tasks, looking
for runnable work waiting while a worker is idle: in a busy worker's next
slot (a thief takes it only after the grace and on its last round), in a
ring no idle worker was woken for (a push whose wakep found a spinner
that then stopped), or in the inject queue. Candidate changes, each kept
only if the trace shows its case and the benches hold:

- a busy worker's next slot taken at once when its owner has run its
  current item longer than a slice (a per worker start time, W_RUN_AT),
  instead of after the grace;
- a spinner that gives up after its rounds rechecks the inject queue and
  every next slot once more (any_work does, but go_idle runs it before it
  sleeps only);
- the monitor's tick also wakes an idle worker for a ring that has held
  items longer than a tick, not only for a deferral.

What none of these fixes: every worker busy computing, the queued task
waits for a handler to end. That is preemption's (step 3), as is
XO_THREADS=1.

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
| 1 | harness p50/p99, 8 and 16 CPU loops | confirmed (CI run 39486203392 against 39465430589, r8g, load 3 to 7: 8 loops p50/p99 60/86 to 0.19 to 0.24/2.4 to 3.1 ms, 16 loops 60/83 to 86 to 0.18 to 0.33/2.7 to 5.1 ms, A and B alike; Go 0.3 to 0.6/7.5 to 12.5; idle unchanged) | | | |
| 2 | ticker repro (late ticks of 20), timer tests | partial (rootpark 394 to 0 to 164 ms, flaky) | confirmed (rootpark 0 ms in every run; TestRootParkHandOff with no known failure) | | |
| 3 | fast request latency behind a slow handler | partial (acceptbusy about 800 to 0 to 2 ms) | | | |
| 4 | the flaky tests, 50 runs each under load | | | | |
| 5 | emit wall time and trace of a cold self build | | | | |
| 6 | dev-goldens timeouts over a day of runs | | | | |
