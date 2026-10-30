# 0421: Go channels in `use go` are receive-only `go.Recv[T]` handles

Status: Proposed (Lang 1, 2026-10-29; the lead agreed with the form; the owner accepts or rejects it, since it adds std/go surface)

## Context

`use go` skips every signature with a channel. The 2026-10-29 survey (87 packages) counts 26 such
skips in the third-party set and a few in std: fsnotify's `Watcher.Events chan Event` and
`Watcher.Errors chan error` fields (the package's whole API is these two fields), go-redis's
`PubSub.Channel() <-chan *Message`, robfig/cron, and
std's `time.After`, `time.Tick`, `Timer.C`. Almost all are channels Go sends on and the caller
receives from. Xo has its own `Chan[T]` (core 7) on its own scheduler; a Go channel lives in Go's.

`use go` builds only with `--backend=go` (core 11). There an Xo task runs on a goroutine, and a Go
call is a plain Go call on it: GoEnter/GoExit (std/rt gointerop.go) count the thread as inside Go
and, under the deterministic scheduler (`xo run --record`, decision 0073), lend its turn only when
callbacks are live (s.cbLive, lendLocked); otherwise the call keeps the turn until it returns.

## Decision (proposed)

1. A Go `<-chan T` or `chan T` given to Xo (a result, a field or variable read) whose element maps
   is a handle `go.Recv[T]` (std/go, a generic struct over an rt value like `go.Any`) with:
   - `fn recv(self) -> T? uses go`: waits for a value; `None` once the channel is closed and empty.
   - `fn try_recv(self) -> T?? uses go`: does not wait; `None` when no value is ready, `Some(None)`
     when closed.
   - `fn each[fx: Effects](self, f: fn(T) uses fx) uses go, fx`: receives until closed.
   Sending, closing and `len`/`cap` are not bound. A `chan<- T` (send only) stays skipped; a
   `go.Send[T]` may follow if a surveyed API needs it. No goroutine pumps a Go channel into an Xo
   `Chan`, and `select` across Go and Xo channels is not offered.
2. **Scheduler** (the lead's question 1). On the Go backend a waiting `recv` parks its goroutine in
   Go's runtime (a channel receive); no OS thread is held and other Xo tasks keep running on their
   own goroutines, exactly as any blocking `use go` call (an `http.Get`, a `sync.WaitGroup.Wait`)
   behaves today. Under the deterministic scheduler a Go call that waits holds the turn, which for
   `recv` would stop the Xo task that is to send (through a Go callback) from ever running: `recv`
   therefore lends its turn before it waits (the lendLocked path, the same as a Go call with live
   callbacks) and its return is a scheduling point (goLeave's admission), so the received value
   joins in a fixed order; `--record` records each received value and `xo replay` returns it
   (value mode, decision 0093), the channel handle itself replaying as a handle call. The deadlock
   report keeps core 7's rule: a Go call waiting on a Go channel is never reported.
3. **Cancellation** (question 2). `recv` and `each` wait with `select` on the channel and on the
   task's context (rt GoContext(tk), whose Done is the task node's cancellation): an Xo scope's
   cancel or a `task.with_timeout` deadline unblocks a pending `recv`, which then raises the task's
   cancellation like any cancellation point (7.2, 7.5). No `recv_timeout(d)` is needed: a timeout is
   a `task.with_timeout` around the receive, as for every other wait in Xo. Inside `defer`
   (shielded) `recv` waits without cancellation, as other cancellation points do.
4. **time.After, time.Tick, Timer.C, Ticker.C** (question 3) are not bound: Xo's clock is a
   capability (`Clock`, the virtual clock of test worlds, the deterministic scheduler's timers), and
   a Go timer channel is invisible to all of it (a test world's clock auto advance would not fire
   it, a replay would not reproduce it). The binder skips channel results and fields of package
   time with "use Xo's clock (decision 0421)". Other packages' channels that a Go timer feeds
   (cron) are bound; their timing is Go's, as any Go call's is.
5. The element conversion is the binder's usual one (mapType); an element that does not map skips
   the channel as today.

## Consequences

- fsnotify becomes usable (`for` over `w.events().each(fn(e) { ... })`), go-redis PubSub receives,
  and the survey's 26 channel skips mostly bind (numbers in the push).
- std/go gains `Recv[T]` (std.md 12) and rt a GoRecv helper; the binding format bumps.
- A program waiting on a Go channel inside `--record` is ordered at each receive, so a recording of
  a program that reads a Go event stream replays those events.

## Alternatives

- **Pump into an Xo `Chan`**: a goroutine per Go channel forwarding into an Xo channel through a
  GoCallback. `select` would work on the Xo side, but it adds a hidden goroutine and buffer per
  channel, receives lose their order relative to other Go calls, and closing and cancellation need
  extra protocol. Rejected for now.
- **Keep skipping**: fsnotify and pub/sub clients stay unusable from Xo.
- **Bind time's channels**: rejected (point 4); Xo's clock and `task.with_timeout` cover the uses.
