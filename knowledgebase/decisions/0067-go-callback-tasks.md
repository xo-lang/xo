# 0067: A Go callback called on another goroutine is a task of its own

Status: Accepted (2026-10-08). Updated by 0073 (deadlocks through Go calls; seed reproducible and recorded foreign callbacks).

## Context

An Xo function passed to Go (`use go`, decision 0041) is converted by the
gobind adapter into a Go func. Go may call it in two ways: on the calling
task's goroutine during the Go call (`strings.Map`, `sync.Once.Do`), or on
a goroutine of its own, while the task runs or after it ended
(`sync.WaitGroup.Go`, `time.AfterFunc`). Decision 0066 took the request's
arena away from callbacks (`GoCallbackTask`), but the callback still ran
with the passing task's thread record, held mutex set, and cancellation
node. On another goroutine that is wrong in every part that keys on the
task:

- Held mutexes: a Mutex the request held looked held by the callback, so a
  callback that waited for it faulted with a false XO-F004 ("called
  re-entrantly in one task") on Go's goroutine, crashing the program; two
  goroutines wrote one held set (data races under `-race`).
- Deterministic scheduler (`xo test`, `--seed`): two goroutines used one
  thread's turn; a Go call that waited for the callback
  (`WaitGroup.wait`) kept the turn.
- Deadlock reports and `xo dump`: the callback's waits were recorded on the
  request's thread.

The golden `test_go_callback_task` shows it: the test task (and a request)
holds Mutex `a` while a `sync.WaitGroup.Go` callback takes `b` and then
waits for `a`. Before this change `xo test` crashed with the false XO-F004
in some runs and `xo test --race` reported data races; after it passes 2
of 2 under `--explore=40`, under `--race --explore=40`, and as a golden
under TestGolden and TestGoldenRace.

## Decision

`rt.GoCallback` decides per call (gobind format version 3):

- On the passing task's goroutine while that task is inside a Go call
  (`thread.inGo > 0` and `curG()` equal to the goroutine recorded when the
  callback crossed into Go): the callback runs as the passing task (without
  the request's arena), like a scope body. Re-entering a Mutex the task
  holds is still XO-F004, as it is a real self deadlock there.
- Otherwise: a child task of the passing task's cancellation node, with a
  thread of its own (kind `callback`), its own held mutex set, counted by
  the deadlock monitor, and started on the deterministic scheduler, where
  it waits for a turn. A thread that holds the turn while inside a Go call
  lends it (`sched.lendLocked`), as does a thread that enters a Go call
  while callback threads are alive; it takes a turn again when the Go call
  returns (`goExit`). A cancellation unwind ends the callback (it returns
  zero values to Go); a fault still panics on Go's goroutine, as before.

Every adapter brackets its Go call with `GoEnter(tk)` and a deferred
`GoExit(tk, name)` (which also turns a Go panic into a fault and now passes
cancellation and exit unwinds through). Goroutine identity is `curG`: the
runtime g pointer from the g register (arm64) or TLS (amd64) in a two
instruction assembly function, a `runtime.Stack` parse elsewhere.

## Consequences

- Held mutex tracking, cancellation (a request's cancellation cancels its
  callbacks), deadlock detection, and `xo dump` see a foreign callback as
  the task it is. Cost: one atomic add per Go call and an allocation per
  callback conversion; a foreign call allocates a task, a thread, and a
  node (as a spawn does); a synchronous call costs a `curG` read.
- Under the deterministic scheduler, the moment a callback thread starts
  depends on Go's goroutine, so interleavings involving foreign callbacks
  are not reproducible by seed. A thread lent to a Go call keeps the world
  from reporting a deadlock (it may come back), so a Go call that waits
  for a callback blocked on the caller's Mutex hangs instead of being
  reported, as any blocking Go call always did.
- Recording (`XO_RECORD`): a foreign callback thread is not a recorded
  scheduler event, so a replay of a trace with foreign callbacks may
  diverge; Go calls were already unrecorded (core 11.3).
- Cached bindings of format version 2 are regenerated.

## Alternatives

- Always a task of its own, even on the caller's goroutine: a synchronous
  callback that takes a Mutex its caller holds would hang silently instead
  of XO-F004, and every `strings.Map` rune would allocate a task.
- Goroutine id by parsing `runtime.Stack` on every call: 1.3 us per call,
  10 us with 40 frames (measured on the host), too slow for per element
  callbacks; kept only as the fallback for architectures without the
  assembly.
- Leave the scheduler (`tk.leave`) around every Go call: makes every Go
  call a nondeterministic point under `--seed`; lending only while callback
  threads exist keeps programs without them reproducible.
