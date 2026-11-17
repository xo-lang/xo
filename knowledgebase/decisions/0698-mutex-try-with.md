# 0698: Mutex.try_with, a lock attempt that never waits (Go's TryLock)

Status: Accepted (owner, 2026-11-17, xw decision #76; proposed by xo-lang-1, task #224)

## Context

Task #224: std/task's last stdaudit gap was Go's `sync.Mutex.TryLock`
(scripts/stdaudit/map.txt). Xo's mutex is scoped (`with`, `with_read`,
decision 0063): there is no unlock to forget, so a bare `try_lock` that
returns a guard has no place in the language. Programs that use TryLock in
Go do so to skip work when another task is already doing it (a cache
refresh, a flush, a best effort stats sample) without queueing behind it.

## Decision

`fn (m: Mutex[T]) try_with[R, fx: Effects](f: fn(var T) -> R uses fx) -> R? uses fx`

- When no writer holds `m`, no writer waits for it and no reader shares it,
  `f` runs exactly as in `with` (the value is moved in, its writes kept, a
  fault in `f` poisons `m`) and the call returns `Some(result)`.
- Otherwise `f` does not run and the call returns `None` at once. A task
  that already holds `m` (inside its own `with` or `with_read`) also gets
  `None` rather than `XO-F004`: like Go's TryLock on a locked mutex, the
  attempt simply fails, and it cannot deadlock.
- A poisoned `m` faults with `XO-F010`, as `with` and `with_read` do.
- `f` has no error type: a body that can fail returns a `Result` as its
  value (`R` = `T ! E`), keeping `R?` unambiguous. Its writes are always
  kept.
- It never waits, so it is not a cancellation point; it is a scheduling
  point like `with` and is in the realtime checker's blocking list
  (it may still run a body that blocks).
- Writers queued for `m` win: a `try_with` while a `with` waits returns
  `None`, keeping writer preference (0063) and the no-starvation rule.

Implementation: native backends call `rt.mutex_try_lock`
(std/native/llrt/mutex.xo `try_lock`, which returns the value list or an
empty list) and share `with`'s body lowering (compiler/ssa/conc.xo
`mutex_body`); the wasm runtime has `$mutex_try_lock` in rt3.xo; the Go
backend calls std/rt `Mutex_try_with`.

## Consequences

- sync.Mutex.TryLock maps to `Mutex.try_with` in stdaudit; std/task has no
  stdaudit gap left.
- Tests: internal/llvm/testdata/golden/mutex_try_with on arm64, LLVM and
  wasm (free, held by another task, inside the task's own `with` and
  `with_read`, a re-entrant `with` in the body, poisoned, reset), with a copy in internal/gen/testdata/golden for the Go
  backend; std/rt TestMutexTryWith.
- RWMutex.TryRLock has no counterpart; a `try_with_read` can follow the
  same shape if a port needs it.

## Alternatives

- `try_lock() -> Guard?`: needs a guard type and an unlock path, which the
  scoped mutex design avoids (0063).
- `try_with` returning `R ! E` with a distinct "busy" error: mixes lock
  contention with the body's own errors; `R?` keeps them apart.
- Faulting with XO-F004 when the task holds `m`: Go's TryLock returns false
  there, and a fault would make the call unsafe in helpers that may run
  inside a `with`.
