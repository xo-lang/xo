# A replay with Go callbacks printed another output and said "identical run"

Status: Resolved (2026-10-08, decision 0073)

Symptom: `XO_SEED=3` runs of `internal/gen/testdata/replay/gocallback`
(four `sync.WaitGroup.Go` callbacks logging into a Mutex beside main)
printed 8 different orders in 8 runs. Replays of `XO_RECORD` traces of
it printed an order other than the recording's, and still ended with
`xo replay: identical run`.

Cause: a callback that Go calls on its own goroutine became a ready
thread, with the next thread id, the moment its goroutine reached
`GoCallback.Begin`, which depends on Go's scheduling. Picks are logged by
thread id, so a replay whose callbacks arrived in another order ran the
recorded ids with other callbacks behind them; nothing in the trace
named the callback, and Go calls are not recorded, so the replay saw no
divergence (the effects and the end matched).

Fix: the scheduler admits callback calls (and lent Go call returns) only
at decisions after Go's goroutines settle, in a fixed order, giving ids
at admission; a recording logs each admission (`c<n>`, effect
`go.callback` with the arguments) and a replay admits the matching call
there (`std/rt/gosched.go`). Second trap met on the way: the deadlock
proof's goroutine classifier treated any goroutine with `time.Sleep`
above an rt frame as a sleeping runtime watcher, which matched a Go call
to `time.Sleep` made inside `Mutex.with` (an rt frame further down) and
reported a false XO-F001 in a replay; only the frame directly below
`time.Sleep` counts now (`TestGoParked`).
