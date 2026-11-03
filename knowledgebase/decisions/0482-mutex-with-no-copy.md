# 0482: Mutex.with without a copy of the protected value

Status: Proposed (Xo Lang 2, 2026-11-03; for the owner, through the lead)

## Context

Core 5.4 on `Mutex[T].with`: "Mutations to the `var` parameter are
written back when the closure returns successfully. If it returns an
error or faults, the protected value is left unchanged." To keep that
promise, the closure the compiler builds catches a fault and hands back
the value it was given (Func.catches, compiler/ssa map.xo closure_with,
decision 0085). The given value therefore stays alive while the closure
changes its copy, so the first write to a heap value inside it copies
that value (a list's or map's storage). A map written under `with` again
and again turns into decision 0291's log form, whose copies are O(1) but
whose put and lookup are slower than the lists'.

Measured on 2026-11-03:

- A 20 line repro, 300,000 writes of `sub[id] = E{...}` into a map of
  maps (XO_PROF samples on darwin/arm64, a profile count, not a timing
  claim): flat local map 7 to 10 samples; the same through `Map.update` on
  a local 18 (after the update fix of the same day, gotchas/var-closure-kept-value-copies-map.md);
  the same inside `Mutex[St].with(fn(var s) ...)` 86 to 90 samples, all in
  the map log's put and lookup.
- std/cookiejar keeps its jar in a `Mutex[JarState]`. After that day's
  fixes it runs at 1.476 of Go's wall (`both`,
  bench/results/std-twin-cookiejar-2026-11-03.md); the map log's put and
  lookup are about 20% of its profile, the largest cost left.
- std has 66 `with(fn(var ...))` closures: http 27, sql 13, websocket 7,
  metrics 6, rate 4, cookiejar 3, task 2, llrt/pg 2, trace 1.

## What "provably cannot fault" would cover

The compiler already knows which functions may fault: `Module.may_fault`
(compiler/ssa passes.xo), from `value_self_faults`, carried to callers.
An operation faults when it is `fault(...)`, a checked `+ - * / %`, shift
or float-to-int (any with a source position), a bounds-checked index, a
call through a function value, or a faulting runtime call; a call faults
when its callee may. A closure whose function may not fault (and cannot
be canceled, prune_unwind's other fact) never reaches its unwind exit, so
keeping the given value buys nothing and the copy can be skipped.

That rule is sound and cheap, but it covers little: `s.count += 1` is a
checked add, and a map put or a list push calls code with checked
arithmetic, so almost every closure that changes a collection may fault.
cookiejar's three closures all may (a `+=` on a sequence number, map
puts, an explicit `fault`). Option A alone would not move cookiejar.

## Options

- **A. Skip the copy when the closure provably cannot fault** (the rule
  above). No semantic change; sound; helps closures that only assign
  fields or call non-faulting code (counters with wrapping ops, flag
  flips, field swaps). Implementation: after prune_unwind, a catches
  closure whose may_fault and cancel facts are false gets its hand-back
  removed before reference counting, so the given value is not kept.
- **B. Go's semantics: a fault leaves the writes made so far.** Errors
  keep today's undo (the closure's `Err` path and XO0540 stay as they
  are), but a fault, which already unwinds the task, no longer restores
  the value. No copy unless the closure can return `Err`. This is what
  Go's `mu.Lock(); defer mu.Unlock()` does on a panic. Every std closure
  above whose error type is Never gains. Cost: a task that faults halfway
  through an update leaves the protected value half changed for the other
  tasks.
- **C. Rust's poisoning: a fault leaves the writes and marks the mutex
  poisoned;** a later `with` on it faults (a new fault code) until the
  program replaces the value (`m.reset(v)`). No copy either; the half
  changed value is never seen silently. A larger surface (a code, a
  method, the spec).
- **D. Keep core 5.4 as it is** and leave std to restructure hot state
  (smaller values under each Mutex, or Atomic and channel designs), with
  today's copy and log form as the price of the guarantee.

## Proposal

A in any case (no semantic change). For the cookiejar class, B, with A
still applying to closures that can return `Err`; the owner chooses
between B, C and D, since B and C change core 5.4's promise for faults.

## Consequences (B)

- core 5.4: "If it returns an error, the protected value is left
  unchanged; if it faults, the writes made before the fault stay (as Go's
  deferred Unlock)." xo explain and std.md's Mutex section say the same.
- compiler/ssa: closure_with keeps the given value only when the
  closure's error type is not Never; the fault path no longer hands it
  back.
- A golden that faults inside `with` after a write and shows the write
  stayed, on every backend; cookiejar's and an http store's --ab and twin
  rows.

## Alternatives

The options above. A copy on first write only on the fault path is not
possible: by the time a fault happens the value has already been changed
in place.
