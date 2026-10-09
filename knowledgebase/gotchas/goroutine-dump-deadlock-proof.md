# Reading goroutine states to prove a deadlock through a Go call

Status: Observed (2026-10-08, decision 0073)

Symptom: the first version of `goQuiet` (std/rt/godeadlock.go) never
confirmed the deadlock of `go_callback_deadlock_real`: the program hung
as before. In `go test ./std/rt`, `TestMonitorGoCallDeadlock` never got
a report either.

Cause: goroutines that can never wake a task looked like moving ones.

- `os/signal.loop` (started by `signal.Notify` in RunMain) sits in
  `signal_recv` with state `[syscall]`, so a rule "a system call may
  return" rejected every real program.
- Under `go test`, the parent test goroutine waits in
  `testing.(*T).Run` on a channel; its stack holds the package's test
  function, so it counted as an Xo goroutine inside foreign Go code.
- Profiler labels are inherited by goroutines a goroutine starts: Go's
  goroutine of `sync.WaitGroup.Go` (and a spawned task under the
  deterministic scheduler, which never cleared its label) carried the
  passing thread's `xo_thread` label, so `lazyStacks` could map a thread
  id to another goroutine's stack.

Fix: classify a goroutine by its frames before its state: frames in
`os/signal.`, `testing.(*M).` or `testing.(*T).Run` mark a helper that is
ignored; then running, runnable, `syscall`, `IO wait`, and preempted
goroutines block the proof. A callback thread labels Go's goroutine with
its own id (`labelThread` in `GoCallback.Begin`), and a spawned task
under the scheduler clears a label inherited from a labeled spawner.
Wait reasons in `runtime.Stack` headers are specific since Go 1.21
(`[sync.WaitGroup.Wait]`, `[sync.Mutex.Lock]`, `[sync.Cond.Wait]`);
`TestGoParked` pins the classification on canned dumps.

Update (Observed 2026-10-08): ignoring every goroutine in `osSleep` was
too broad. The scheduler's settle loop (`waitGoSettled`) also sleeps
there, while a callback call that a fired Go timer started waits to be
admitted, and a test world reported a correct program as deadlocked.
A goroutine with `waitGoSettled` on its stack now counts as moving, and
the report also needs a still scheduler (see `go-runtime-timer-linkname.md`).
