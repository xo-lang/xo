# A wasm wake resumes every blocked task, and a repeatable socket call subscribes again

Status: Observed (2026-11-03, decision 0447 R2f)

## Symptom

http_h2_config as a wasip2/wasm command trapped in `wasi_sock_try_read`
with "cannot lend resource: store handle limit exceeded" while an HTTP/2
connection sat idle waiting for a keepalive PING.

## Cause

rt3.xo's `$wake` (a park signal, a channel send) resumes every blocked
task, and each retries its call. compiler/wasm/rt9.xo's waits subscribe
to a pollable (a new handle) and record it in the wait table, which only
`$io_poll` (when no task can run) and `$sk_unwait` empty. HTTP/2's reader
task waits in `wasi_sock_readable` and its connection fiber signals a
park the reader waits on; each signal resumed the reader, which took the
resumption for "readable", signaled the fiber back, and subscribed
again. The two tasks kept each other runnable, `$io_poll` never ran, and
the pollables filled the store's handle table.

## Fix

`wasi_sock_readable` marks its wait in the connection record (offset 48)
with the wait table's generation (`$wgen`, offset 52), which `$io_poll`
and `$sk_unwait` count up when they drop pollables. A resumption with
the same generation is a wake for someone else: it blocks again on the
pollable it already has. (Not a lookup of the pollable's number: a
dropped handle's number can be reused.)

The other repeatable calls (read, write, accept, connect) still
subscribe again after such a wake; each wake adds one pollable until the
next `$io_poll`, which is bounded unless tasks wake each other in a
loop. Give a new waiting call the same generation check if it can be
woken in a loop.
