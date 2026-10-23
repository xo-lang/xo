# 0350: Deterministic runs in the native runtime

Status: Accepted (owner, 2026-10-22). Stages 1 and 2 follow the release;
stage 3 (det by default for native `xo test`) returns to the owner with
`xo bench --ab` numbers.

## Context

Core 7.4 promises a deterministic scheduler: one task runs at a time and
a seeded generator picks the next at every scheduling point, so a seed
reproduces an interleaving. `xo test --explore=N` reruns a passing
concurrent test under N more seeds, `xo run --seed=N` (and `XO_SEED`)
runs a program on that scheduler with the real clock, and `xo run
--record` (core 11.3) logs every pick and effect result for `xo replay`.

All three exist only on the Go backend (std/rt sched.go, replay.go,
gosched.go: about 2,800 lines). Since decisions 0248 and 0270 native is
the default, so these flags silently pick `--backend=go`, and core 7.4
says a native program "may print differently from run to run". This is
the audit's item 5 of what still needs the Go backend. It matters more
than its size suggests:

- `xo test` natively runs on the real scheduler on a pool of threads, so
  a racy test passes or fails by luck, and the seed printed for a failure
  means nothing natively.
- The Go backend lags native on the workloads that need concurrency
  testing most (bench/results/go-backend-gap-2026-10-22.md: median 2x,
  up to 24x before that day's fixes), and its runtime is a different
  implementation: a Go-backend `--explore` tests std/rt's channels and
  Mutex, not std/native's.
- Contract fuzzing and `xo dump` are the other Go-only pieces; this
  decision covers only the scheduler and recording.

The native runtime already has most of the parts:

- the test world (decision 0083; std/native/llrt testworld.xo, twrun.xo)
  with a virtual clock that moves when every context waits (twrun.xo
  kick), a seeded PCG (testworld.xo W_TWRNG), and FakeNet's in-memory
  listeners;
- fibers with park and wake as the only ways a task stops and resumes
  (sched.xo fiber_park, fiber_wake; pool.xo push, find_work);
- `XO_THREADS=1` (poolsetup.xo) and `--no-preempt` (decision 0286 step 3);
- the deadlock check (pool.xo dl_check) and its XO-F001 report.

What is missing is a picker: today the next fiber is whatever the
worker's ring, the inject queue, a steal, or the poller yields, and a
fiber keeps running until it parks.

## Decision

### 1. A det build flavor, not a runtime switch

Deterministic runs are a build flavor, like `--race`:

- `xo test` (stage 3), `xo test --explore`, `xo test --seed`, `xo run
  --seed` and `xo run --record` build the program with det on;
- every other build is unchanged and pays nothing: no flag test, no hook
  call, no extra code;
- the flavor is part of the build and cache keys, so a det build and a
  normal build of one program are separate entries.

With det on, the compiler (compiler/ssa) emits a call of
`rt.det_point(kind)` at each scheduling point core 7.4 lists that has an
inline fast path:
- uncontended `Mutex.with` and `with_read`;
- `Atomic` operations;
- channel operations that do not block;
- `task.is_canceled()` and `task.stop_requested()`;
- effect calls (before the call).

The points that already go through the runtime's slow path (a park,
`spawn`, `await`, `sleep`, `Once.get`, a blocking channel operation) call
the picker from the runtime itself. A det build also turns off the
preemption loop polls and asynchronous preemption, as `--no-preempt`
does: under core 7.4 a CPU-bound loop keeps the turn, as on the Go
backend.

`XO_SEED=N` on a binary built without det has no effect natively, as
today; core 7.4 says so. Section "Alternatives" covers the runtime switch.

### 2. The picker: one worker, a ready set, the seeded generator

A det program runs on one worker thread with the main fiber (the pool
starts no other workers; roots other than main are refused, see 5):

- **ready set.** pool.xo `push` (every wake: channel, Mutex grant, scope
  end, timer, poller) appends the fiber to a det ready list instead of
  the ring or the run-next slot. The list keeps arrival order, so the
  pick depends only on the seed and the program.
- **pick.** `det_point` puts the running fiber at the end of the ready
  list and draws `pcg % n` (the test world's PCG, seeded by `--seed`,
  default 1, as on the Go backend); if the draw is the running fiber, it
  returns at once with no switch. `find_work` in det mode is that draw:
  no steal, no inject queue (the poller's results go through `push`), no
  spinning.
- **nothing ready.** In a test world the virtual clock moves to the next
  timer (twrun.xo kick), and that timer's wakes enter the ready list in
  timer order. With the real clock (`xo run --seed`) the worker blocks in
  the poller until a timer, I/O or signal wakes a fiber. Those wakes enter
  the ready list in the poller's order. That order is reproducible only as
  far as the timing is, which is the Go backend's rule ("as long as timer
  and I/O timing does not decide them").
- **deadlock.** No ready fiber, no timer, and no I/O or signal wait means
  every task waits for good: dl_check's XO-F001 report, as today.

The Go backend picks among threads by `rng.Uint64() % n` over a list in
the same arrival order, so with the same generator a test that touches
only scheduling points sees the same sequence of choices on both
backends. Byte-equal schedules across backends are not promised: the
Go backend's points include Go callbacks (gosched.go), which native does
not have. The goal is the same guarantee, not the same trace.

### 3. Explore and seeds in `xo test`

`xo test --explore=N` reruns each concurrent test that passed (one that
spawned or awaited) under seeds 2 to N+1, in the test binary's own
process, resetting the test world between runs as `testing.run_main`
does. The first failing seed is printed with `test "x" seed S` for
pinning, as core 7.4 says. `--seed=S` and a pinned seed run that seed
only.

### 4. Recording and replay

`xo run --record=FILE` on a det build logs:
- each pick (by task id: ids are given in spawn order, and only the
  running task or an outside event spawns, so they match between runs);
- each outside event at the decision that admitted it (a timer firing, a
  socket ready, a signal);
- each effect call's operation, encoded arguments and encoded result,
  with its Xo location.

The effect boundary is the capability methods' native implementations
(std/native's os, fs, net, http, proc, time modules). The compiler
generates an encoder and a decoder for each effect result type, as derive
Json does, and the trace uses replay.go's format and version, so `xo
replay` reads either backend's trace for the same program. Replay
returns the recorded results without doing the I/O and delivers the
events at the same decisions. `use c` calls follow core 11.3's rule
(boundary values replayed, a C handle reruns), which native has never
recorded (10.1).

### 5. What det mode refuses

- threads that C code starts and that call back into Xo (`use c`
  callbacks on foreign threads): det mode reports them as an
  unsupported fault naming the callback, since they cannot be admitted
  at a decision without Go's settle step (gosched.go has one for Go);
- `--race` together with det (ThreadSanitizer needs real threads;
  one-thread det has no data races to find);
- `--no-gc` keeps working (det changes scheduling, not memory).

## Stages

1. **Picker and `xo run --seed`.** The det flavor (build key, the
   flag), `det_point` emitted at the inline points, the ready list and
   draw in pool.xo, one worker, preemption off, the real clock path.
   Proof: a new golden whose output depends on the interleaving (two
   tasks appending to a shared list through a Mutex) prints the same
   bytes 20 times for one seed and differs between some two seeds; the
   existing goldens run under `--seed=1` and still match want.txt
   (output independent of interleaving must not change). core 7.4's
   native paragraph and 0248's flag list change.
2. **`xo test --seed` and `--explore` natively.** The test world's
   clock drives the picker; explore reruns in process. Proof:
   internal/gen's concurrent goldens (test_explore, test_deadlock,
   test_mutex_read, test_once, ...) and TestTestSeedReproduces run on
   the native backend: test_explore's planted race fails under some seed
   of `--explore`, the printed seed reproduces it, and a pinned good seed
   passes.
3. **Native `xo test` on the det scheduler by default**, as core 7.4
   says for every `xo test`, with `--real` (decision 0120) choosing the
   thread pool. Owner decision at that stage: it changes how native tests
   run (one thread) and may slow CPU-heavy tests; the stage measures the
   repository's test suites before and after with `xo bench --ab`.
4. **`--record` and `xo replay` natively.** The effect codecs, the
   trace writer and reader shared in format with replay.go, crash
   bundles' effect rings (`XO_RING`). Proof: every record/replay test of
   cmd/xo passes with the native backend, and a trace recorded natively
   replays to the same output.
5. **Remove the Go-backend selection** for `--explore`, `--seed` and
   `--record` (0248's list keeps `--hybrid` and `--holes`), with the
   spec and the website updated.

## Cost when off

None at run time: a program built without det has no `det_point`
calls, no ready list, and the pool's code paths are unchanged (the det
branches in `push` and `find_work` are behind a constant the det flavor
sets, folded at build time, as `tracerec.built()` is for a release
build's trace recorder, decision 0240). The compiler
gains one flag in its build key. Stage 1 proves "no output change" for
normal builds with scripts/identity over the goldens and compiler/xo.

In a det build, each inline point costs a call (det_point returns at
once when the ready list is empty, the common case in a single task
program) and one PCG step when it is not. Det builds run on one thread,
so a CPU-parallel program runs at single-thread speed, as on the Go
backend's deterministic scheduler today.

## Consequences

- The Go-only flags' last scheduling users go native; the Go backend is
  then needed for `use go`, `--hybrid` and `--holes` only (with contract
  fuzzing and `xo dump` as separate items).
- Native tests become reproducible by seed (stage 3), and a failing
  seed printed by CI reproduces on the Mac.
- Two runtime modes to keep correct: det runs exercise a different
  `find_work`. The golden runs under `--seed=1` (stage 1) keep the det
  path tested on every change to the scheduler.
- Det builds are separate cache entries: a first `xo test --seed` of a
  program builds it again.

## Alternatives

- **A runtime switch (`XO_SEED` honored by every native binary).** It
  matches the Go backend's "any built binary" wording. But every inline
  scheduling point (uncontended Mutex, Atomic get) would carry a load and
  a branch in every program, on the paths decision 0286 and the Mutex
  work spent the most effort to shorten. Rejected unless the owner wants
  `XO_SEED` on release binaries; the flavor can still be built with
  `xo build --seed-support` (a later flag) for that use.
- **Keep the Go backend for these flags.** Today's state: tests explore
  a different runtime than the one that ships, and the Go backend's gap
  grows with each native-only optimization.
- **Systematic exploration (all interleavings, partial order reduction)
  instead of random seeds.** Stronger for small tests, but core 7.4
  specifies seeds, and seeded random exploration finds most bugs at a
  far lower cost. It can come later on top of the same picker (the
  picker is the only part that changes).
- **Record by syscall (rr style) instead of at capability methods.**
  Records everything, including C libraries' syscalls, but its traces
  are tied to the binary and the kernel, and it cannot replay on another
  machine or under a debugger of our own. Capability methods are the
  boundary core 11.3 already defines.
