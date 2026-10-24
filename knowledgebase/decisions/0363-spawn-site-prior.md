# 0363: A prior for a new spawn site that takes a Mutex (initial placement)

Status: Proposed (Xo Lang 3, 2026-10-24; for the owner's Sunday review; nothing built).

## Context

ADR 0359 (AD) classifies a spawn site by what its finished tasks cost and keeps
a site of short tasks on the spawner. It learns from task ends, so it helps a
program that spawns bursts repeatedly and does nothing for a one-shot burst
that finishes before the site has learned. The two release-table rows that are
lock-bound are exactly such bursts: conc `rwmutex` (one burst of 64 tasks,
37 ms, 3.9x Go's wall) and `mutex` (8 tasks, 11 ms).

Measured 2026-10-23/24 on the Mac (arm64, 12 threads, quiet window), all by
`xo bench --ab` or counters, patches kept on local branches:

- `XO_THREADS=1` runs the 64-task rwmutex repro (rwrep, 20 bursts) in 126 ms
  against 671 ms with 12 workers, with 0 parks against 5.06M. Every access
  parks exactly once (writers 1.276M of 1.28M, readers 3.78M of 3.84M; no
  spurious re-parks). The structure is a writers-preferred lock with a writer
  present most of the time; threads only fight for one lock word.
- A learned lock-take classification of the site (wip/lock-classify): rwrep
  0.386, mrep 0.544 (better), but rwmutex 1.008 and mutex 0.998: the repeated
  bursts learn after the first, the one-shot rows never do.
- Reader spin before parking (wip/mutex-spin, owner declined the knob): all
  rows equal or worse at N=16 and 64; mutex 1.23 to 1.27 worse.
- Mid-burst throttle, no stealing once N tasks are parked on a Mutex
  (wip/mid-burst): rwmutex 37.6 ms to 215-231 ms. By the first parks the burst
  is spread over the workers, and starving the rest queues work behind parked
  lock holders.
- Variant G of 0359 (no wake per spawn, static) reproduced the serial run:
  rwmutex 0.37 of main's wall, mutex 0.67; the owner chose AD over G (0359
  records G as an alternative).
- The park/signal pair is about 15% of CPU; a wake is 0.8 us (a macOS XO_PROF
  artifact had shown 31%, gotchas/xo-prof-macos-overstates-ulock-wake.md).

So the lever is the initial placement of a site's first burst, before any task
has finished.

## Proposal

A compile-time prior for a spawn site, with AD as the run-time correction.

1. The checker marks a `spawn` site `lockish` when its closure (directly, and
   through the calls it can resolve in the same module, to a bounded depth of 2)
   calls `Mutex.with`, `with_read`, `lock`, or `rlock` inside a loop. This
   is a flag on the site (a bit next to the site's loc in the spawn call),
   not new syntax.
2. A new `lockish` site starts in the small state AD already has (its burst
   stays on the spawner, no wake per spawn, run serially at the join). AD's
   average then corrects it: a task that runs long (over SITE_SMALL_NS, 20 us)
   per take marks the site big, so a lock held across real work still spreads.
3. A site not marked `lockish` starts as today (big until it learns).

Runtime cost: one flag load per spawn. No new runtime state (AD's site table).
No language change; a hint a user can see only through `xo explain`/trace.

## Experiment (to run only after the owner has seen this)

Compiler change in compiler/check or ssa (the site flag), runtime change in
pool.xo scope_add_at (initial site state). Verdict by `xo bench --ab`, 20
rounds, quiet window, on the eight conc rows (rwmutex and mutex must move
toward 0.37 and 0.67 of main; pmap, spawn, fanout, pingpong within noise),
CRUD and the HTTP benches (p99 included), plus the TestRuntime* tests and a
--race pass in the gate. Also the Go-twin comparison in
bench/results/release-vs-go-*.md.

## What would make it a no

- rwmutex/mutex do not move: the cost is in the lock, not placement.
- Any other conc row or CRUD/HTTP p99 gets worse than noise: a flagged site
  that scales (a read-mostly lock where parallel readers do run in parallel)
  loses its parallelism. The correction by AD takes one burst, so a one-shot
  read-mostly burst would pay. Guard to test: a read-mostly variant of rwmutex
  (1 write in 1,000).
- The static flag is wrong too often: a closure that locks once or locks
  through a helper the checker cannot see (miss, not harm) or a closure that
  takes a Mutex in a loop around long work (flag false positive, corrected
  after one burst). Count both on std/ and bench/ programs before building.

## Experiment results (2026-10-24, Xo Lang 3; status stays Proposed)

Prototype on local branch `lockish` (saved as refs/recover/2026-10-24/lockish,
4cbdd841f, never pushed): `lockish` in compiler/ssa/conc.xo sets spawn flag
bit 2 for a closure whose own body calls `Mutex.with` or `with_read` inside a
loop; `pool.site_prior` seeds that site's average to 1 ns. A = origin/main
561482d1c, B = the prototype, both xos clean, XO_INLINE_STRICT=1, checksums
equal. `xo bench --ab`, 20 rounds, quiet window, wall B/A (95% interval):

| Program | B / A | Verdict |
|---|---|---|
| conc mutex | 0.654 (0.640 to 0.663) | better |
| conc rwmutex | 0.252 (0.250 to 0.255) | better |
| read-mostly rwmutex (1 write in 1,000) | 0.568 (0.507 to 0.609) | better |
| pmap | 0.990 (0.958 to 1.030) | noise |
| fanout | 0.998 (0.985 to 1.013) | noise |
| spawn | 1.014 (0.978 to 1.044) | noise |
| pingpong | 1.006 (0.992 to 1.014) | noise |
| CRUD HTTP, 5 x 10 s, 32 workers | 142,576 vs 142,727 rps; p99 661 vs 671 us | noise |
| worker pool, 100 rounds of work per job | 0.788 (0.694 to 0.803) | better |
| worker pool, 1,000 rounds per job | 1.416 (1.405 to 1.426) | worse |
| worker pool, 10,000 rounds per job | 6.173 (6.141 to 6.214) | worse |

TestRuntime* (internal/llvm, internal/repotest) pass, and --race builds of
mutex, rwmutex, spawn, fanout and pmap give the same checksums.

Survey (std/, compiler/, bench/): no real site in std/ or compiler/; bench/
has the 2 intended sites and 13 worker-pool sites (a lock around a result
store after real work); misses through a same-module helper are 4 in
internal tests, 1 in compiler, 0 in std.

Reading: the lock-bound rows move to or past the targets (0.67, 0.37) and
nothing else moves, but the worker-pool false positive is the third "no"
criterion above, and it is not corrected after one burst: the worker pool's
one burst is the whole run, the first task starts on the spawner and drains
the shared queue before the other seven exist, so a pool with 10,000 rounds
of work per job runs fully serial (6.2x slower, 468 ms against 76 ms).
Worker pools with a lock around a result store are the common shape (13 of
the 15 bench sites), so the flag as defined fails the experiment. What the
data leaves open: a tighter flag (the lock is the only work in the loop body,
or the loop body between lock takes has no call), which mutex and rwmutex
satisfy and these pools do not.

### Tighter flag (2026-10-24, same day)

The pools above fail the third criterion, so the flag was narrowed
(`lockish` 07f3c4286, saved as refs/recover/2026-10-24/lockish, never
pushed): a spawn is flagged only when a loop of its closure takes a Mutex and
every call outside the closures built in that loop is a Mutex take or an
Atomic method. A worker pool calls `queue.recv()` and its work function
between lock takes, so it is no longer flagged; conc `mutex` and `rwmutex`
(the lock is the work, `steps()` runs under it) still are. Same method, xos
clean, checksums equal, `xo bench --ab` 20 rounds, quiet window, wall B/A
(95% interval), B = the tighter flag:

| Program | B / A | Verdict |
|---|---|---|
| conc mutex | 0.650 (0.636 to 0.660) | better |
| conc rwmutex | 0.255 (0.251 to 0.257) | better |
| read-mostly rwmutex (1 write in 1,000) | 0.575 (0.539 to 0.631) | better |
| pmap | 1.002 (0.996 to 1.008) | noise |
| fanout | 0.997 (0.985 to 1.011) | noise |
| spawn | 0.996 (0.966 to 1.025) | noise |
| pingpong | 0.998 (0.992 to 1.010) | noise |
| worker pool, 100 rounds per job | 1.006 (0.998 to 1.021) | noise |
| worker pool, 1,000 rounds per job | 1.006 (0.996 to 1.018) | noise |
| worker pool, 10,000 rounds per job | 0.996 (0.989 to 1.009) | noise |

The two lock-bound rows keep their gain (the same as the first flag, 0.654
and 0.252) and the three pools that were 0.79 to 6.17 are back to the
parent. Other checks with the tighter flag (same day): TestRuntime* (internal/llvm,
internal/repotest) pass; --race builds of mutex, rwmutex, spawn, fanout,
pingpong, pmap, the read-mostly guard and the three pools run with no race
report and the parent's checksums; the CRUD HTTP bench (5 x 10 s, 32 workers,
arm64, quiet window) gives 143,344 rps (A) against 142,833 rps (B), p50 200
against 203 us, p99 669 against 655 us, p99.9 1,051 against 1,031 us, no
errors, within noise (the server CPU per request, sampled by ps, reads 22.87
and 25.35 us and varied from 21.2 to 25.8 us across the A runs of the
earlier run, so it is not a result). Not run: the 13 real bench pools
(bench/paper, bench/planted-xo*, bench/xo/v0.4 worker pools), which need
network access or inputs the harness does not supply; the three synthetic
pools stand in for them. What the narrower flag
misses: a lock in a loop with a non-lock call in it (such a site is a pool
or a mixed workload, not one this experiment moves), so it is a flag for
programs whose loop is the lock.

## Consequences

If it works: the two lock-bound rows approach the serial run without a knob
and without waiting for a program to burst twice. A spawn site taking a Mutex
in a loop would run serially at first by default, which is the right
default for the Mutex semantics Xo has (writer preferred, park on contention)
and wrong only for a lock held briefly around parallel-friendly work.

## Alternatives

- Learn faster inside the burst (the lock-take classification, the mid-burst
  throttle): measured above, they act after the burst is spread.
- G (no wake per spawn for every site): static and simple, but costs parallel
  rows; the owner chose AD.
- A user hint on `spawn` (`spawn_heavy`, listed in 0359): the owner
  would write it; this proposal derives the same bit.
- Spin before parking: measured, worse.
- Make parking cheaper: at most 15% of CPU; no lever.
