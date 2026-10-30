# 0429: an upgraded connection is in flight at a stop

Status: Accepted (Xo lang 5, 2026-10-30; the lead approved the recommendation the same day). Implementation after the seed cut from main on 2026-10-30.

## Context

Std 4.4 says that on a stop `serve` "stops accepting, waits for in flight requests, and returns". It did not say whether a connection handed over by a 101 counts as in flight; std's hand-overs are std/websocket's `upgrade` and the reverse proxy's pass-through of a target's 101 (std/rt http_proxy.go, native http.xo), and this decision covers both. The backend gap survey of 2026-10-30 (bench/survey-2026-10-30/chathub) hit this. Reading the code showed that the two backends differ.

- Native: std/native/http.xo `respond` runs the hand-over function `f` inside the connection's task. On a stop, llrt hserve.xo `srv_stop` closes only idle connections, and `srv_wait` waits for every connection until the stop's grace period ends (the serving task is canceled). So `serve` waits for `f`.
- Go backend: std/rt/http_net.go stops with `http.Server.Shutdown`. net/http ignores hijacked connections in Shutdown, so `serve` returns while `f` still runs.

## Decision

An upgraded connection is a request in flight until its function returns, on every backend:

- On a stop, `serve` waits for each hand-over function to return, within the stop's grace period, as it waits for any request. Once the grace period ends, the remaining functions' tasks are canceled: their WebSocket reads and writes fail with `WsErr.Net(Canceled)`.
- A long lived handler watches `task.stopping()` and closes its connections with 1001 (going away), as gorilla's examples close on shutdown.
- Go backend: std/rt counts each hijacked hand-over function in a WaitGroup and waits on it after `Shutdown` returns, with the same grace period rule. Native code stays as it is.

## Consequences

- Spec std 4.4 and 35 state the rule.
- A golden on every backend: a server with a WebSocket handler, a client connected, then a stop. The handler sees `stopping()` and closes with 1001, the client reads `Closed(1001, ...)`, and `serve` returns only after the handler did.
- std/changes/websocket.md and http.md entries; std-coverage survey 2026-10-30, SPEC item.
- A handler that ignores stops holds `serve` until the grace period ends. That is the same as a slow request.

## Alternatives

- Go's net/http behavior (a hijacked connection is not tracked). Rejected: `serve` would return with handlers still running, so a program's cleanup after `serve` (closing the store a handler writes to) races them. Native already waits.
- Closing upgraded connections at once on a stop (1001 sent by the server). Rejected: the handler cannot finish what it was doing (flush a queue, send a last message). Watching `stopping()` gives it that chance.
