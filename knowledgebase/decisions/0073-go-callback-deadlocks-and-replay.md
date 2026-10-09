# 0073: Deadlocks through Go calls, and seed reproducible, recorded Go callbacks

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

### 2. Foreign callbacks under the deterministic scheduler and in traces

`std/rt/gosched.go`. Once an Xo function crossed into Go under the
scheduler (`NewGoCallback`, which numbers each callback in the order the
running thread passes them), the scheduler is armed:

- **Admission at decisions.** A callback call that arrives on a goroutine
  of Go's does not become ready at once: it waits in a pending list (its
  goroutine blocked, no thread id yet), as does a thread whose lent Go
  call returned. At each decision the scheduler first lets Go's
  goroutines settle (every goroutine but the deciding one parked,
  sleeping, or in a system call or I/O; at most 200 ms) and then admits
  what arrived in a fixed order: returning threads by id, then callback
  calls by callback number and encoded arguments; thread ids are given
  at admission. So whatever Go code the program's own Go calls set off
  has arrived before the seeded choice, wherever the Go runtime happened
  to run it.
- **Go call returns are scheduling points** once armed: callbacks Go
  started during the call (`WaitGroup.Go`) join there, so they can
  interleave with the caller as a spawned task does.
- **A Go call that blocks for a callback.** A thread holding the turn in
  a Go call keeps it; when a callback arrives meanwhile, a helper waits
  until Go's goroutines settle and, if the thread is still in its Go
  call, lends its turn (decision 0067's lending, now at a settled point
  instead of the instant of arrival).
- **Recording and replay.** Each admission is a scheduler stream token
  `c<callback>` and an effect `go.callback` of the new thread with the
  callback's Xo arguments (gobind format 4 converts the arguments before
  `cb.Begin`, which takes them through a closure that runs only for a
  foreign call under the scheduler). A replay admits, at that token, the
  pending call of that callback whose encoded arguments match the
  recorded effect, waiting for it to arrive; returns of Go calls are
  matched by the picks that name their threads.

Before: the replay program `testdata/replay/gocallback` printed 8
different orders in 8 runs with `XO_SEED=3`, and replays of its
recordings printed other orders than the recording while reporting
`identical run`.

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

- Part 2: `internal/gen` `TestReplayGoCallback` (same seed, same order
  over 6 runs for 4 seeds, at least 2 orders; 4 recordings each with 4
  `go.callback` entries replay to the same output, also with -race) and
  `std/rt` `TestSchedGoCallbackSeed` (a test world). A deadlocked
  recording through a Go call replays to the same XO-F001.
- Part 2 limits: Go code woken by a Go timer or I/O of its own arrives
  when it arrives (as timers and I/O always did under `xo run --seed`);
  a settle gives up after 200 ms when some goroutine keeps running (a
  busy Go goroutine makes every decision of an armed run that slow and
  the order unreproducible); calls of one callback with equal arguments
  on several goroutines are interchangeable; a Go call that never blocks
  but runs while other threads take turns (lent) returns at the next
  settled decision. Programs that never pass an Xo function to Go keep
  their exact interleavings and traces; armed ones get a scheduling
  point at every Go call return, so their interleavings for a seed
  differ from before this decision.
- Cost (part 2, deterministic scheduler only): one goroutine dump per
  decision while Go code may have run since the last one (a Go call, a
  callback's end, a lent thread, a pending arrival); nothing on real
  runs. Real runs pay one closure (on the stack) per callback call.

## Alternatives

- Treat every thread in a Go call as blocked when its callbacks are:
  reports a Go call that reads the network while its callback waits (the
  non deadlock the golden checks).
- Only check the goroutine of the Go call: a Go library goroutine woken
  by the network or a timer may still release it.
- Record each Go call's site in `GoEnter`: about 200 ns per Go call, paid
  by every `strings` call; the profile label costs once per thread.
- Admit a callback the instant it arrives (decision 0067): the decision
  index it joins at depends on Go's goroutine timing.
- Admit callbacks only when every Xo thread is blocked: reproducible
  without dumps at Go call returns, but a callback would never run beside
  ready threads, so `--explore` would miss most of its interleavings.
- Let a replay call the callback itself from the trace: Go calls run for
  real in a replay, so Go would call it too, and its other effects on Go
  state would be missing.
