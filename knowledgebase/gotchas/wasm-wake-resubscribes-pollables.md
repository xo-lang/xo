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

`$sk_wait` records, in the waiting task (words 208 and 216), the wait
table's generation (`$wgen`, which `$io_poll` counts up when it drops the
table) and the wait's deadline, and the task in its wait table entry;
`$sk_unwait` (a close, and a deadline set on the connection) clears the
word of each task whose entry it drops, so only those tasks subscribe
again (a global count would make every waiter on every connection
subscribe again at each deadline set, which servers do per request). On a resumption,
`$sk_reblock` (from `$sk_enter`, so every repeatable call: read, write,
accept, connect, a lookup's next, and `wasi_sock_readable`) blocks the
task again on the pollable it already has while the generation is the
same, the deadline has not passed and the task is not canceled; the call
returns as when it suspends (`$sk_enter` returns 2; a write keeps its
progress). Not a lookup of the pollable's number: a dropped handle's
number can be reused. The first fix (2026-11-03, b84a0b69c) did this for
`wasi_sock_readable` only, in the connection record; the Lead asked for
every call, since a server under load is where tasks wake each other in
a loop.

A deadline set while a call waits must end that wait: `wasi_sock_deadline`
woke the tasks so they would repeat their call under the new deadline, and
the re-block kept the old one instead (conn_deadline hung, 2026-11-03); it
now drops the connection's waits with `$sk_unwait`.
