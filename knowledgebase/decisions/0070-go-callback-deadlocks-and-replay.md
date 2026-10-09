# 0070: Deadlocks through Go calls, and seed reproducible, recorded Go callbacks

Status: Accepted (2026-10-08). Closes the two gaps decision 0067 left.

## Context

Decision 0067 runs a Go callback that Go calls on a goroutine of its own
(`sync.WaitGroup.Go`, a timer) as a child task with its own thread. Two
consequences remained:

1. A task inside a Go call is running Go code, which no Xo wait record
   describes. When the Go call waits for such a callback
   (`WaitGroup.Wait`) and the callback waits for a Mutex the task holds,
   the program hung: the real run's monitor counted the task as running,
   and test worlds treated a thread lent to a Go call as one that may come
   back. The goldens `go_callback_deadlock_real` and
   `test_go_callback_deadlock` hung (the test until its 60 s limit)
   before this decision.
2. Under the deterministic scheduler, the moment a callback thread
   started depended on Go's goroutine, so interleavings with foreign
   callbacks were not reproducible by seed, and `--record` did not log
   them, so a replay could diverge.

## Decision

### 1. Deadlocks through Go calls (XO-F001)

A thread inside a Go call counts as blocked when it waits only for its
own foreign callbacks and they are stuck (`std/rt/godeadlock.go`):

- **Counting.** A foreign callback thread records the thread that passed
  it (`thread.cbOf`); each thread counts its live ones (`cbN`); the
  monitor of a real run keeps the live callback threads. A fixpoint
  (`goWaitersOf`) finds the threads inside a Go call (`inGo > 0`) whose
  live callbacks are all blocked on Xo primitives or are such threads in
  turn. The monitor suspects a deadlock when blocked threads plus these
  cover every live thread; a test world's scheduler, when no thread is
  ready and every lent thread is one of them.
- **Proof from the goroutines.** Counting cannot tell a Go call that
  waits for its callback from one that reads the network. Before it
  reports, the runtime reads every goroutine's stack
  (`runtime.Stack(all)`, `goQuiet`): each must be parked where only
  another goroutine of the program can wake it. Goroutines with Xo frames
  parked in the Xo runtime pass; goroutines inside a Go call pass only
  in a `sync` package wait (`WaitGroup.Wait`, `Mutex.Lock`, `RWMutex`,
  `Cond.Wait`, `semacquire`); anything running, in a system call, in I/O
  wait, sleeping in Go code, waiting on a channel or select in Go code
  (which may be a timer), or a goroutine with no Xo frame (a library's
  own) means no report. Signal handling goroutines and the go test
  driver are ignored. The real run's monitor looks again (backing off to
  2 s) while the counts hold; a test world's scheduler polls (2 ms to
  100 ms) until a thread moves or the check passes.
- **Report.** The thread in the Go call becomes a wait record of kind
  `Go call` that names the innermost Go function and points at its
  callbacks, so the wait cycle shows the edge:

      test blocked in Go call sync.(*WaitGroup).Wait at main.xo:16 for callback 2
      callback 2 blocked in Mutex.with at main.xo:15 on a mutex made at main_test.xo:2, held by test since main.xo:14

  The location and Go function come from the goroutine profile: a
  thread that passes a callback labels its goroutine with its id
  (`NewGoCallback`, once per thread, real runs and the scheduler only),
  and a callback thread relabels Go's goroutine with its own id (Go's
  goroutine inherits the passing goroutine's labels). Spawned tasks
  under the scheduler drop an inherited label. `xo dump` shows a task in
  a Go call as `in a Go call`, with its callback count.

## Consequences

- `go_callback_deadlock_real` exits 70 with the cycle; it first checks
  that a Go call sleeping in Go (`time.Sleep`) while its callback waits
  is not reported. `test_go_callback_deadlock` fails its deadlocked test
  with the cycle and passes the sleeping one under `--explore=10`; both
  are also run under -race (TestGoldenRace). `std/rt`
  `TestMonitorGoCallDeadlock` and `TestGoParked` cover the monitor and
  the goroutine classification.
- Limits (the provable case only):
  - A Go call that waits for a callback of another task, or on a Go
    channel, select, or timer, is never reported (it hangs, as any
    blocking Go call did).
  - Go state that a Go timer would release (`time.AfterFunc` calling
    `WaitGroup.Done`) is invisible: such a program is reported although
    the timer would have ended the wait.
  - Tests with real capabilities (`xo test --real`, no scheduler) do not
    detect it.
- Cost: none on Go calls of threads without live foreign callbacks (one
  atomic load); one goroutine label per thread that passes a callback in
  a real run; the stack read happens only when every task already looks
  blocked.

## Alternatives

- Treat every thread in a Go call as blocked when its callbacks are:
  reports a Go call that reads the network while its callback waits (the
  non deadlock the golden checks).
- Only check the goroutine of the Go call: a Go library goroutine woken
  by the network or a timer may still release it.
- Record each Go call's site in `GoEnter`: about 200 ns per Go call, paid
  by every `strings` call; the profile label costs once per thread.
