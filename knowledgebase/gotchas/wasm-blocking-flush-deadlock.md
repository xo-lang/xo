# A blocking WASI call stops every task in the instance

Status: Observed (2026-11-03, websocket_tls_deadline as a wasip2 command)

Symptom: websocket_tls_deadline hung (wasmtime: no exit in 2m) in about
1 of 20 runs on main and 4 of 7 on a branch whose module was byte
identical, so it looked like a regression until the base rate was
measured.

Cause: Xo's tasks in a wasm module share the one thread, and a WASI
`blocking-*` call returns only when the host is done. rt9.xo's TCP close
called `output-stream.blocking-flush` before dropping the stream. When
the peer reading those bytes was a task in the same program (both ends
of a connection in one golden) and had not read yet, the flush waited for
a read that could only happen after it returned. A slow remote peer
stalled every task the same way, without a deadlock.

Fix: rt9.xo never calls a blocking WASI function on a task's path. A
close starts `output-stream.flush` and asks check-write at once: nothing
unsent (the flush ended) or a failed stream drops now, as before; else
the stream and socket go to the closing list, which `$io_poll` polls
beside the task waits and drops from once the flush is done. After main
ends `$sk_closing_drain` waits at most 2 s and leaves what is left to the
instance's end. Golden net_close_flush (1 MB queued at the close, read
after it in the same program) is the proof. Measure a flaky wasm golden's
rate on its base (-count=20) before bisecting it.

Second trap, found by the fix's first version: wasmtime's resource-drop
of an output stream whose write is still in flight waits in the host for
that write. The first version kept a connection with nothing unsent in
the closing list (its socket open), so in conn_deadline the peer's drop,
with megabytes queued to that open socket, waited forever: 20 of 20 runs
hung after the last line before "done". Only drop a stream whose flush is
done or failed; never drop the leftovers at main's end.

Known issue (open, a wasmtime limit, 2026-11-03): a peer that never
reads keeps a closed connection's socket and stream in the closing list
for as long as the program runs, so a long running server keeps one
entry, its socket and the host's buffer per such client. rt9.xo warns
once on stderr at 1,024 entries (CLOSING_WARN) and keeps them. Two ways
to free them were tried with golden net_close_stuck (100 connections
closed with a write in flight while the client holds them unread):

- an expiry (the write deadline, else 30 s), then tcp-socket.shutdown(both)
  and the drop: the drop still waited for the write, and the program hung
  (10 of 10);
- the shutdown, then the drop once check-write reports the write failed:
  no hang, but the write never failed while the peer held the
  connection, so all 100 entries stayed until the client closed
  (counted with a logging build).

wasmtime's drop of a TCP output stream waits for a pending write; see
wasmtime PR #14461 "wasi: restore cancellation of pending TCP writes on
stream drop" (open since 2026-10-01). Re-test when wasmtime merges it or
ships an equivalent: with a drop that cancels the write, an entry past
its expiry can be dropped and the warning becomes a bound.
