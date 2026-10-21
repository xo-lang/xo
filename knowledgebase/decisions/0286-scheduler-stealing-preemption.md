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

Step 2's measurement (Mac, 2026-10-20, xo trace of the cpuburst server on
12 cores, step 0 in): at 12 /cpu loops dial to response p50/p99 58.6/66.1
ms, wake to run p99 102.8 ms over 188 wakes; at 18 loops 66.9/81.6 ms, wake
p99 157.1 ms over 223 wakes (idle time per worker 0 to 1.3 s over the
whole run). The long waits match the handlers' length, about one handler,
not a core left idle beside queued work, so none of the candidates above
has a case in these traces and none lands now.

## Step 3: preemption at loop iterations (2026-10-21)

The owner's direction: a cheap poll of a flag a timer sets, as Go's
cooperative preemption before its signals.

- The flag: the static word 76 (std/native/llrt/pool.xo W_PREEMPT; the
  pool's static words moved from 46 to 75 and 76), 1 while the monitor
  asks some worker to yield. Each worker stores when it started its
  current fiber (W_RUN_AT, run_fiber) and whether it is asked
  (W_PREEMPT). The monitor (mon_main, which ticked only for deferrals)
  is also woken when work is queued with every worker busy (wakep's
  saturated path, preempt_arm), and keeps ticking every 1 ms while a
  worker runs and work waits; it asks every worker that has run one fiber
  SLICE_NS (10 ms, Go's forcePreemptNS), and clears the flag when none is
  asked.
- The poll: the SSA builder adds rt.preempt_poll(rt.static_word(76)) at
  every loop iteration of the program's own code (loop_tick, where the
  budget poll is), on arm64. The arm64 emitter lowers it to `ldr w16,
  [addr]` and `cbnz w16` to a stub after the function that calls
  xo_preempt_stub and comes back (keeping x30 in a frame record where it
  holds the return address, a leaf or a block without the frame); the
  LLVM emitter to inline asm with the same three instructions (x16 and lr
  written). xo_preempt_stub (sched_asm, and a weak naked LLVM function)
  saves x0 to x17, x18 off Apple, the flags, and q0 to q31 around the
  runtime's xo_x_preempt: the poll clobbers nothing, so the register
  allocator sees no call in the loop.
- xo_x_preempt (pool.xo preempt): when this worker is asked, the running
  fiber yields (sched.xo fiber_yield, state F_YIELDING); its scheduler
  puts it at the inject queue's tail once it has switched away (no park
  holds it, so no waker races the queueing), and the worker's own queue
  runs first. A thread without a fiber does not yield (a root with
  XO_MAIN_FIBER=0); main on its fiber does.
- XO_PREEMPT=0 asks no worker (the kill switch, toolchain.md).
- Not yet: amd64 (native and LLVM), whose stub saves rax, rcx, rdx, rsi,
  rdi, r8 to r11, the flags, and xmm0 to xmm15, with Windows' shadow
  space; std's own loops (only the program's own code polls); wasm has
  one thread and no polls.
- Checks: testdata/sched/preempt (XO_THREADS=1: the two threads spin on
  a flag only a queued task sets; it ends only by preemption, and hangs
  with XO_PREEMPT=0); acceptbusy at XO_THREADS=1 and the cpuburst trace
  at 12 and 18 loops; the compute benches' instruction counts against
  main for the poll's cost.
- First measurements (Mac, 2026-10-21, native arm64): testdata/sched/
  preempt ends its five rounds in 63 to 90 ms (about one slice a round)
  and hangs with XO_PREEMPT=0. The poll is two instructions in the loop
  (`ldr w16, [x11]`, `cbnz`; the flag's address hoisted out of it). A
  call free loop of about five instructions (`s = s + i * 3` 100 million
  times) took 6 to 13% longer, best of 7 runs: 34.8 to 35.8 ms against
  37.7 to 39.4 ms. That is the cost to decide on before this lands; the
  ways to cut it, in order: poll every Nth iteration of a counted loop
  (a counter the loop has already, tested with `tst` on its low bits:
  one instruction, not two), poll only the outer loop of a nest, or
  asynchronous preemption by a signal (Go since 1.14: no cost in the
  loop, a safe point table instead).
- The placements measured (Mac, 2026-10-21; compiler/ssa budget.xo
  PREEMPT_MODE, one xo built per mode from the same tree;
  bench/perf/portable, native arm64, instructions retired from
  /usr/bin/time -l, the minimum of 5 runs, against mode 0, no polls):

  | workload | 1 every iteration | 2 every 64th (countdown) | 3 outermost loops | 4 skip short constant ranges |
  |---|---|---|---|---|
  | arith32 | +5.2% | +5.4% | +0.1% | +5.9% |
  | arith8 | +8.2% | +8.3% | +0.1% | +8.2% |
  | u64 | +4.4% | +4.4% | -0.6% | +5.0% |
  | divmod | +2.6% | +2.7% | +0.2% | +2.6% |
  | shift | +3.8% | +3.9% | -0.2% | +3.8% |
  | conv | +3.5% | +4.8% | +0.0% | +3.5% |
  | str_hash | +2.4% | +2.0% | -0.2% | +2.5% |
  | unix | +5.4% | +11.1% | +0.1% | +5.6% |
  | checked | +2.6% | +2.6% | -0.0% | +2.5% |
  | float32 | +3.9% | +4.1% | -0.0% | +4.0% |
  | all | +3.5% | +3.9% | -0.1% | +3.6% |

  Best wall time moved within the runs' noise for every mode (-5% to
  +2%, two outliers of +13% in modes 2 and 3 on one workload each): the
  M-series runs the load and the branch beside the loop's work. The
  countdown (2) costs as much as the poll it replaces (a subtract and a
  branch against a load and a branch) and more where it adds a live
  register (unix); skipping short constant ranges (4) finds none of
  these loops. Polling only the outermost loop of each function (3)
  costs nothing here, and leaves an inner loop that runs long under an
  outer one that does not iterate unpreempted.
- HTTP under callgrind (linux/arm64 in Docker, callgrind-docker.sh
  MODE=short: the server's instructions per request, N=30, one repeat),
  mode 0 against mode 1: plain 24,216 against 24,099 (-0.5%), tls
  36,591 against 37,147 (+1.5%, within what TLS's random keys move). The
  servers' loops are std's, which do not poll; only the program's own
  code does.
- Why std does not poll yet, and what it would take (the owner's
  question, 2026-10-21): without polls in std, a long loop inside std
  (sorting or hashing a large input, json.parse of a big body, a regexp
  over a long string, crypto over a large buffer) runs its whole length
  before the handler that called it can yield, which is a case
  preemption is meant to cover. The runtime (std/native/llrt) must never
  poll: it holds the pool's, a park's, and the inject queue's spin locks,
  and a fiber that yields holding one leaves every other worker spinning
  on it. The rest of std is only safe to poll if the runtime never calls
  it with a lock held, which nothing checks today: the guard is Go's
  m.locks, a count per worker of the runtime locks held (pool.xo plock,
  klock, lock and their unlocks add and subtract it, about one store
  each), and preempt yields only when it is 0. PREEMPT_MODE 5 polls every
  module but llrt, to measure the cost; its numbers are below.
  Mode 5 (2026-10-21, after runtime_mod: native/llrt, native/rt*, and
  native/sys_* never poll): bench/perf/portable +3.5% instructions, as
  mode 1 (its loops are the program's own), wall within noise; HTTP
  callgrind against mode 0: plain 24,131 against 24,216 (-0.3%), tls
  36,308 against 36,591 (-0.8%): polling std costs nothing measurable on
  these servers. So std's loops can poll once the lock count guards the
  runtime's calls into std.
- acceptbusy at XO_THREADS=1 (worst fast request of three rounds, three
  runs each, native arm64): mode 0 447, 662, and 18 ms (a handler's 250
  ms or more whenever both threads compute); mode 1 29, 21, 21 ms; mode 5
  21, 11, 11 ms.
- Open: the slice; a global flag against a per worker one (every busy
  thread takes the slow path once a tick, about 1000 a second); yielding
  to the inject queue's tail against the ring's.

### Step 3: the owner's choice (2026-10-21) and what landed

The owner chose (a+): every loop iteration polls, in the program's own
code and in std, never in the runtime, guarded by a per worker count of
the runtime locks held, with XO_PREEMPT=0 as the kill switch.

- The builder polls in every module but the runtime's (ssa budget.xo
  runtime_mod: native/llrt, native/rt*, native/sys_*), test bodies
  included; the measurement modes (PREEMPT_MODE 0 to 5) are gone.
- Every spin lock of the runtime calls pool.held(1) after taking it and
  held(-1) before releasing it (30 acquire and 14 release sites: the
  pool, park, inject, fiber cache, scope and node locks, the poller's
  descriptor locks, the heap's, h2srv's, stdio's, tzhost's, proc's,
  stop's, libcmem's, the test world's). The count is a word of the
  worker record (W_LOCKS); preempt yields only at zero and otherwise
  leaves the request for the next poll after the release. Under
  XO_LOCKCHECK=1 a fiber that switches away with the count not zero is
  a fatal "lock held" error (pool.xo run_fiber), so an unbalanced site
  shows at once.
- The monitor asks a worker to yield only when work waits, no worker is
  idle, and no other can start (otherwise wakep wakes or starts one): a
  spawner that spins was preempted the moment it spawned, before an idle
  worker could steal its child, so the child ran on the spawner's thread
  (internal/llvm TestRuntimeTasks' magazines, 2026-10-21).
- amd64 (2026-10-21): the amd64 emitter lowers a poll to `testl` and
  `jne` to a stub that steps over the red zone (not on Windows) and calls
  xo_preempt_stub; LLVM on x86_64 the same in inline asm. The stub (ssa
  preempt_stub_amd64) keeps rax to r11, the flags, and xmm0 to xmm15 in a
  frame it aligns itself, Win64's shadow space below them. The test
  writes the flags; the poll is at an iteration's start, where none is
  live.

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

## Addendum (2026-10-21): the poll's cost under load

The +3.5% measured for step 3 came from loads where the poll flag was
rarely set. Under steady load (Docker linux/arm64, 12 vCPUs shared with
the client; the CRUD server, bench/perf/http, under a list-only load: 1024
keep-alive connections each looping GET /users over 1,032 users), the
flag stayed set: the monitor asks a worker whenever work waits and none is
idle, and one global word told every worker's polls to call the stub,
which saves all registers before pool.preempt finds this worker was not
asked. xo_preempt_stub was 26% of the server's CPU (flat) and the server
did 49,124 requests in 8 s with XO_PREEMPT=1 against 68,844 with
XO_PREEMPT=0.

Fix: the stub first loads this worker's ask (its record through
xo_tlocal: x16 only, plus x17 on Darwin, whose TLV thunk changes it; the
LLVM poll's clobbers gain x17) and returns saving nothing when it was not
asked; W_PREEMPT counts the asks out (the asked worker's preempt, or the
monitor's withdrawal of an ask unserved for a slice, takes one back)
instead of staying set while work waits. Same load, one run each, back to
back (Docker varies about 20% between runs, so compare within a run):

| XO_PREEMPT | before | after |
|---|---|---|
| 1 | 46,903 requests (1,203 us CPU each) | 51,894 (1,027 us) |
| 0 | 57,022 (1,019 us) | 61,506 (892 us) |

Then the remaining cost, with the fix on main (b7f56be67): the same load,
the client now recording latency, each server twice in one container
run (preemption off; on; a preempted fiber at the tail of its own
worker's ring instead of the inject queue's; a 20 ms slice):

| server | run 1 | run 2 |
|---|---|---|
| XO_PREEMPT=0 | 52,135 requests, p50 142 ms, p99 510 ms | 46,191, p50 163 ms, p99 539 ms |
| XO_PREEMPT=1 (main) | 52,419, p50 148 ms, p99 427 ms | 47,941, p50 152 ms, p99 538 ms |
| own ring's tail | 42,617, p99 786 ms | 45,985, p99 537 ms |
| 20 ms slice | 45,404, p99 733 ms, max 2.2 s | 40,090, p99 933 ms, max 2.0 s |

Preemption as on main now costs nothing measurable on this load
(throughput and p99 within run to run variance, CPU per request equal);
the "15%" seen before compared against an older build. Requeueing on the
worker's own ring and a longer slice were both worse, so the inject
queue's tail and 10 ms stay.

## Addendum (2026-10-21): the polls' cost in loops

Xo lang 4's comparison (bench/results/compare-2026-10-21.md) showed the
loop rows slower since step 3. Against the same compiler with no polls
(native arm64, exact instructions under callgrind; LLVM, darwin user
time): sieve 2,111 M against 1,682 M (+25.5%), sort +11.7%, nbody +9.0%,
fib unchanged (no loops); LLVM sieve 0.18 s against 0.10 s, collatz 0.58
against 0.47. Making the test cheaper (9ccc0f0e5: the flag's address one
loop invariant constant, no memory clobber at a poll, LLVM's test in IR
with only the stub's call as asm) changed nothing measurable: the cost
is the load and branch per iteration in inner loops of six to eight
instructions, and on LLVM a loop with a call path is neither vectorized
nor unrolled. 9ccc0f0e5 also fixed a real bug: a poll did not end the
reuse of the thread local table's address within a block, so a task
resumed on another worker could use the old thread's table
(TestPreemptMigrate). Decision 0318 proposes preempting by a signal
instead of a test in every loop.
