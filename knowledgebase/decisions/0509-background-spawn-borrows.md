# 0509: A scope's s.spawn_background borrows its captures too (amends 0507)

Status: Accepted (owner, 2026-11-05). Proposed by Xo Lang 1 the same day at the lead's request.

## Context

Decision 0507 lets a closure literal passed to its own scope's `s.spawn`
borrow its captures of `let`s and non-`var` parameters declared before
the scope: no retain when the closure is made, no release when its task
ends, and the scope keeps each borrowed variable alive past its join
(a share after every scope_close). It left `s.spawn_background` owned:
a background task runs past the body's last statement, until the close
cancels it, and 0507 rested on "no task outlives its scope" without a
test of the background close paths.

Xo Lang 4's golden conc_scope_bg_outlive (internal/llvm/testdata/golden,
2026-11-05) now tests that rule for background tasks on every close
path: the body completing (mode 0), `return`, `break`, `?`, a failed
child and `s.cancel()` then a normal end (mode 1), a fault in the body
and in a child, and a body unwound by an outer cancel (mode 2). Each
scope has four background tasks whose work the cancel cannot cut short
(a shielded deferred sleep, a compute loop after it sees the cancel, a
channel receive, a group task started from a defer during the close).
On arm64, `--race` and LLVM at XO_THREADS 1, 2 and 8 with XO_FREE_CHECK
0 and 1, and on the Go backend at GOMAXPROCS 1, 2 and 8, every path
reports all five done and none late, right after the scope and 60 ms
later; a scope_close with its mode 0 wait removed is caught (late 3).

So the property 0507 needs, that every task the scope's own spawn starts
ends before the close returns, holds for background tasks as it does
for foreground ones.

## Decision

Decision 0507's rule applies to `s.spawn_background` as it does to
`s.spawn`: a closure literal passed directly to `s.spawn_background` on
the innermost enclosing scope's own variable borrows its captures that
meet 0507's conditions 2 to 4 (declared before the scope; a `let`, or a
parameter or receiver that is not `var`; not move only). Everything else
in 0507 is unchanged: the scope keeps the borrowed variables alive past
every close, the shares at the spawn stay, and a closure held in a
variable, a `task.Group`'s `spawn_background`, a `var`, or a spawn in a
nested scope or closure keeps owning its captures.

The change is one condition: the borrow scan (compiler/ssa conc.xo
scoped_spawn_lit) accepts `spawn_background` as well as `spawn`, and
`spawn` no longer skips the borrow for a background task.

## Consequences

- A scope's background tasks that capture shared values do no count
  traffic for them per task, as 0507 gave foreground tasks.
  bench/perf/micro13 spawnbg100k (100,000 background tasks, each
  `total.add(i % 7 + xs.len())` on a captured `Atomic[Int]` and a
  captured `List[Int]`; a local bench commit, not on main), `--ab` on
  the linux/arm64 offload host, 3 rounds, 0507's main (f702a22bb plus
  the bench) against the branch: wall 0.757 (0.656 to 0.84, better), CPU
  1.023 (0.861 to 1.167, within noise), max RSS 1.0. As with 0507, the
  gain is in the spawning thread's waiting, not in work.
- New golden conc_scope_bg_borrow: background tasks that
  read their borrowed captures after the body's last statement (a loop
  until cancelled, a shielded defer that sleeps and then reads), 10,000
  background tasks on one captured value, `s.cancel()` then a normal
  end, an early return, a `?`, an inner scope unwound by an outer
  cancel, and the owned cases (a `var` reassigned in the scope, a held
  closure, a Group's spawn_background). Every captured value is read
  after the scope, so a missed retain shows as freed or reused memory. It
  passes on arm64 and LLVM, 3 runs each at XO_THREADS 1, 2 and 8 with
  XO_FREE_CHECK=1 (linux/arm64, 2026-11-05), with identical output; the
  conc goldens and conc_scope_borrow pass on both backends, and TSan
  TestGoldenTasks passes (256 s, linux/arm64).
- The risk is the same as 0507's and depends on the same rule: if a
  close path ever returned before a background task ended, a borrowed
  capture could be released under it. conc_scope_bg_outlive now guards
  that rule for background tasks on every backend.

## Alternatives

- Keep background tasks owned (0507 as accepted): costs the background
  spawner the same contended counts 0507 removed for foreground ones,
  for no safety the scope does not already give.
- Borrow in `task.Group` spawns too: a Group can be stored and passed
  around, so its tasks' scope is not the spawning block's; out of scope
  here as in 0507.
