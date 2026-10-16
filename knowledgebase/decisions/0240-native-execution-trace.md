# 0240: A scheduler execution trace for native programs

Status: Proposed (2026-10-16, Xo lang 3, from Xo Lang 2's stub; hook
points agreed with Xo lang 4, owner of the scheduler)

## Context

The sampling profiler (0238) says where CPU time goes and the line
tables (0239) name the lines. Neither shows why a concurrent program
waits: which task parked on which channel or mutex, who woke it, how
long it sat runnable before a worker took it, when workers idled, what
the monitor and the poller did. Diagnosing the scheduler today means
XO_RTSTATS totals and reading pool.xo. Go answers this with its
execution trace; agents and the owner want the same for Xo, readable
both in a timeline viewer and as text.

The owner's rules for the profiler apply (decision 0238's addendum,
2026-10-16): development builds keep it, `xo build --release` leaves it
out unless asked (`--trace-support`), and a written file is created new
(O_EXCL), never through a symbolic link, mode 0600. The lead asked that
the binary record raw events and `xo` turn them into the outputs, which
fits either outcome of 0245, and that the raw header be shared with the
profiler.

## Decision

1. **Switch.** `XO_TRACE=FILE` (and `xo run --trace=FILE`, `xo test
   --trace=FILE`) records an execution trace of the run into FILE in
   the raw format below. `xo trace FILE [--binary PROG] [--json OUT]
   [--text]` writes the Chrome trace event JSON (opens in
   ui.perfetto.dev and chrome://tracing) and a text summary. `xo run`
   and `xo test --trace` call it themselves (OUT next to FILE, the
   summary on standard error), so one command gives both.
2. **Events** (32 bytes each: time in ns since the trace's start, kind,
   reason, worker, the context's address, one argument):
   - spawn: parent, child, the spawn site (task_spawn's `lp`/`ln`
     location, written once per site) and the task's function;
   - run and stop: a worker starts or stops running a context
     (pool.xo run_item and park_wait's resume);
   - park, with the reason: chan send, chan recv, select, mutex, join,
     sleep, net (poller wait), http (hserve, h2srv), test world, other;
   - wake: waker, wakee, handoff or not (pool.xo signal, the one wake
     path); deferred wakes (defer_wake when it defers) and their flush
     (mon_scan and defer_check calling wakep, take_local);
   - worker idle (go_idle before the futex wait) and wake (wakep's
     futex_wake);
   - monitor tick (mon_main after mon_scan: the count it acted on);
   - poller events (poll.xo dispatch: the ready count) and timers fired;
   - task end.
3. **Park reason** is a context word, C_WHY (ctx.xo, C_SIZE 336 to
   344), set by each park's caller (chan.xo, selects.xo, mutex.xo, the
   joins in tasks.xo and pool.xo, poll.xo, hserve.xo, h2srv.xo,
   twrun.xo) and read by the park hook; park_wait_n keeps its signature
   (Xo lang 4's preference: no extra register on every park).
4. **Recording.** Off: static word 67 (rt.xo's table) is 0; every hook
   is a load of it and a branch, no call. On: trace.xo maps a pool of
   1 MiB chunks at start (XO_TRACE_MB, default 64), and each worker
   record carries its current chunk (a new worker word, W_SIZE grows in
   pool.xo and poolsetup.xo); a full chunk takes the next from the pool
   with one atomic add, and when the pool is spent events are counted
   as lost (reported in the summary). The monitor and threads without a
   worker record share one chunk claimed per event with an atomic add.
   No hook allocates, locks, faults or makes a system call; none sits
   in sched.xo, rc.xo or alloc.xo (the no allocation check). The clock
   is sysio.mono_ns.
5. **Writing** at the end of main and at os.proc.exit (as the
   profiler's finish): the header, then each chunk. The raw header is
   shared with the profiler's raw output (0245, if approved): magic
   `XORAW1\0\0`, kind (1 trace, 2 cpu, 3 heap), version, the binary's
   LC_UUID or build ID, its load address, the start's monotonic and
   wall times, the target, lost counts, then the sections. The file
   rules above (O_EXCL | O_NOFOLLOW, 0600).
6. **`xo trace`** (selfhost/xo): reads the raw file and, for names and
   lines, the binary's symbols and DWARF (selfhost/macho's readers and
   dwline's parser, as 0245 plans for `xo prof`). JSON: one track per
   worker thread with each run as a slice named by the task's spawn
   site and function, flow arrows from a wake to the run it caused,
   instant events for monitor ticks, poller events and worker idling,
   and one track per task with its states (running, runnable, parked
   with the reason). Summary: time parked by reason, the top wakers and
   wakees by spawn site, runnable latency (wake to run: median, p99,
   max), worker idle time, deferred wakes and their flush delay, the
   longest parks with their sites, lost events.
7. **Gating** (the owner's rule): the recorder (trace.xo and the hooks'
   bodies) is in development builds; `--release` builds fold
   `trace.on()` to false unless `--trace-support` (the SSA builders'
   build constant folding of decision 0246), so the hooks and trace.xo
   are not linked. XO_TRACE on a release build without it prints
   `xo: built without the trace recorder; rebuild with --trace-support`
   and runs normally.
8. **Platforms**: darwin and linux, arm64 and LLVM backends; Windows
   when its profiler is (0238's Windows item); the Go backend keeps
   runtime/trace.

## Consequences

- Off cost: one static word load and branch per spawn, run, park, wake,
  idle and monitor tick. The bar, as the profiler's: within about 1%
  on bench/perf conc, spawn and pingpong, at XO_THREADS=1 too (spawn is
  lean since 97267570, so a fixed cost shows more), measured against
  the same build with no hooks; bench/results/trace-off-2026-10-NN.md.
- On cost: a clock read and a 32 byte store per event; reported, not
  bounded.
- Binary size: trace.xo's recorder in development builds (the writer
  of JSON is in `xo`, not the binary); nothing in release builds
  without `--trace-support`.
- Tests: internal/llvm TestTrace: a program with a channel ping-pong,
  a mutex, a sleep, a select and a join, traced on arm64 and LLVM; `xo
  trace` gives JSON that parses, every reason appears, each park has
  a later run of the same task, wakes point at runs; no file without
  XO_TRACE; an existing file or a symbolic link is refused.

## Alternatives

- Go's binary trace format and `go tool trace`: a moving format tied to
  Go's runtime states; the Chrome JSON opens in Perfetto with no Go
  toolchain.
- Writing JSON from the binary: the writer, names and lines in every
  development binary; the lead and 0245 point the other way.
- A parameter for the park reason: a register on every park and a
  signature change on many callers; the context word costs one store
  where the reason is known.
- OS tracers (dtrace, perf, ETW): per platform, privileged, and blind
  to Xo's tasks.
