# A sender cancelled inside a multi-piece write leaves a torn frame

Status: Observed 2026-11-17 (std/jsonrpc stress on linux/arm64 host 2, task #196)

Symptom: std/evidence/jsonrpc/stress failed within seconds with
`frame: jsonrpc: no Content-Length header` and then
`fault: XO-F001: deadlock: every task is blocked`, on arm64 and LLVM, on
the Mac and on host 2 (6 of 6 runs of `stress 0 64`). Every answer that
arrived was right.

Cause: `Conn.call` wrote its request in the calling task. The stress
program's transport writes each frame in random pieces, one channel send
each, and every send is a cancellation point; a call whose task was
cancelled (the program's "slow" calls, cancelled after 0 to 2 ms) unwound
between two pieces. The rest of the frame never went out, so the peer's
Content-Length reader read the next message's bytes as a header, failed,
ended its read loop, and closed the connection's pending calls on one
side while the other side waited for answers that could not come. A
Mutex around the write keeps messages from interleaving but does not stop
the holder from unwinding partway. A `defer` is shielded but faults past
5 s (XO-F005), so it cannot shield a write to a slow peer.

Fix: a writer task owns the transport (std/jsonrpc `write_loop`, started
by `serve`): senders queue the message and wait for its result, and a
cancelled sender stops waiting while the writer finishes the frame, as
the Go SDK's jsonrpc2 checks its context only before a write. Any std
code that writes one message as several cancellable steps on a shared
stream needs the same shape (or a single write call that cannot unwind
partway). Test the case with a write function that blocks between its
pieces until after the cancel (the corpus test "Conn: a call cancelled
while its request is half written still writes all of it").
