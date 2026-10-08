# A request to a fake listener before `serve` started failed or deadlocked

Status: Observed (2026-10-08, notes-service test "a listener on :0 reports
its address"); fixed the same day.

Symptom: in a test,

```
let l = os.net.listen(":0")?
let srv = http.server(l, routes())
scope s {
  s.spawn_background(fn() { srv.serve() })
  let resp = c.get("http://${l.addr()}/health")?
}
```

failed with `XO-F001: deadlock ... task 2 blocked in serve`. Without the
`?` the request itself was `Err(Connect("no fake handler ..."))`.

Cause: two runtime bugs in `std/rt/testing.go`.

1. The body runs before the spawned task, so the request reached the
   in memory listener before `serve` had registered the router, and the
   fake network refused it. A real socket would have queued the connection
   in its backlog after `listen`.
2. The body then ended, which cancels background tasks; task 2 started
   already canceled and blocked in `memListener.serve` (`blockOn(waitServe)`)
   without checking `tk.Canceled()`. The deterministic scheduler only wakes
   cancelable waits that were blocked when the cancel happened, so nothing
   woke it.

Fix: the fake dispatch waits on the listener's `served` event (wait kind
"connect (the listener is not served yet)") when `serve` has not started,
like a backlog; `memListener.serve` does not block when the task is already
canceled. Rule for runtime code: check `tk.Canceled()` (or use
`event.wait`, which does) before any `blockOn` whose only waker is
cancellation. std.md 8.1 documents the backlog behavior.
