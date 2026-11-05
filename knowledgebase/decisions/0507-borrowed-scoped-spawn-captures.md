# 0507: A scoped spawn borrows the captures that outlive its scope

Status: Proposed (Xo Lang 1, 2026-11-04; for the owner)

## Context

`Closures passed to spawn capture by value` (core 7.1): making the
closure retains each captured value into its environment, `spawn`
shares the environment (decision 0058: a shared count is negative and
changes atomically), and the task's end releases each capture again.
When every task of a scope captures the same value, those two counts
are atomic read-modify-writes on one cache line from every worker.

bench/perf/micro13 spawn100k (100,000 tasks, each `total.add(i % 7)` on
one captured `Atomic[Int]`) is 1.14 to 1.27 times Go's time
(bench/results/release-vs-go-v0.39.0-precut.md). A perf profile on the
linux/arm64 offload host, 2026-11-04 (40 rounds, xo 0.38.1, `perf record
-g`):

| share | where |
|---|---|
| 20.9% | the Handle drop of the captured `total` (rc_defer, the task env's drop, then an ldaxr/stlxr loop on the shared count) |
| 17.1% | `__aarch64_ldadd8_acq_rel` (8.8% from run_claimed: `total.add` and the scope's pending count; 5.0% scope_add_at) |
| 13.1% | xo_stack_switch |
| 5.5% | pool.scope_add_at |

The Mac's XO_PROF profile agrees: the spawning thread is the bound (1,250
of 1,313 ms), workers are idle 61% of the time. Go's closure holds a
pointer and counts nothing.

Core 7.1 already guarantees what a borrow needs: "No task ever outlives
its scope." When the body completes, the scope waits for every task
(background ones after cancelling them); when it exits early (`return`,
`?`, `break`, an error or a fault) it cancels them and waits for them to
unwind. So a value that is alive, unchanged, from before the scope block
until after it, outlives every task the scope's own `spawn` starts.

## Decision

A capture of a closure literal passed directly to `s.spawn` (a
foreground task) is **borrowed** when all of these hold:

1. `s` is the scope variable of the innermost enclosing `scope s { ... }`
   block of the call (not a `task.Group`, not `os.tasks`, not a Scope
   that came from a parameter or a field).
2. The captured variable is declared outside that scope block (a local
   or parameter of an enclosing block of the same function).
3. The variable is not assigned anywhere inside the scope block,
   including inside closures and nested blocks there (a `var` assigned
   only before or after the block qualifies; any assignment inside, a
   `var` passed as a `var` argument or used as a `var` receiver inside
   (`v.push(x)`, which writes the value in place), disqualifies it).
4. Its type is not move-only (core 7.1: "Move only values are moved").
5. The closure literal itself is not stored or copied: it is the spawn's
   argument expression (a closure value held in a variable is not
   borrowed from).

A borrowed capture:

- is stored into the environment without a retain, and the
  environment's drop does not release it (the environment drop helper of
  that closure skips the slot: the borrowed slots are a property of the
  closure literal, fixed at its one spawn site);
- is shared (marked, decision 0058) at the spawn as today, so a task
  that copies it (stores it in a channel, a Mutex, a struct it returns)
  retains it atomically and owns that copy;
- keeps its variable alive until the scope block's join: the compiler
  adds a use of the variable after the block, so a last use inside the
  block never moves it into the environment (insert_rc's last-use move
  would otherwise make the environment the owner).

Anything else captured stays as today (retained and released).

## Consequences

- A scope whose tasks capture shared values does no count traffic for
  them per task: spawn100k loses the 20.9% drop and the spawner's retain
  (the --ab result goes here before acceptance).
- A missed case in the rule is a use after free. The tests (internal
  arm64 and the conc goldens, plus TSan TestGoldenTasks and
  XO_FREE_CHECK=1) cover each exclusion: a `var` reassigned inside the
  scope while tasks read it; a capture made by a Group's spawn; a closure
  held in a variable and spawned; a task that sends its borrowed capture
  through a channel that outlives the scope, and one that returns it as
  its result; a fault in the body while tasks run; a scope inside a loop
  whose captured variable is declared in the loop body (outside the
  scope); a capture whose last use is the spawn.
- The LLVM and Go backends: LLVM shares the SSA, so it gets the same;
  the Go backend has no counts.

## Alternatives

- Cache line padding or batching in the scope record (the 5.5% of
  scope_add_at): smaller, and independent; measured separately.
- Borrowing in every closure whose callee is known not to keep it
  (decision 0337 does that for callback arguments): spawn's tasks run
  concurrently, so it needs the scope's join, which only `s.spawn` of
  the enclosing scope gives.
- Including `s.spawn_background`: core 7.1 has the scope cancel its
  background tasks and then wait for all tasks, so they too end before
  the variable does, but the runtime's close paths (scope_close modes 0,
  1 and 2) are not yet shown to join a background task in every one;
  left out until a test proves each path, then a follow-up decision.
- Leaving it: spawn100k stays 1.14 to 1.27 of Go, and every
  worker-pool scope pays two atomics per task per captured shared value.
