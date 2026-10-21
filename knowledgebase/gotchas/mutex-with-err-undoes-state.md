# Returning Err from a Mutex.with closure undoes its changes

Status: Observed (2026-10-21, Xo Lang 3)

Symptom: TestGoldenRun/websocket flaked under load (about 1 run in 40
with 16 CPU spinners): the read after a close frame printed `close 1006
(abnormal closure): unexpected EOF` or a connection reset instead of
the close error again. Alone it always passed.

Cause: spec core 5.4: a `with` closure's changes to its `var` value are
written back only when it returns successfully. std/native websocket's
read stored the sticky error (and advanced its buffer) and then returned
`Err`, so both were undone. The next read normally re-parsed the same
close frame still in the restored buffer (same output, by luck); when
the message and the close frame came in separate segments, the restored
buffer lacked the close frame and the read went back to the closed
socket. The same pattern was in websocket's write (close_sent),
BodyReader.read, and PartReader; std/native/sql's driver Tx.commit and
rollback set `done` and give the connection back before returning the
driver's Err.

Fix: return the outcome as the closure's success value and apply `?`
outside (`let outcome = m.with(fn(var s) -> Result[T, E] { ... }); outcome?`),
the pattern 5.4 documents. To find others: a `with(fn(var ...) -> T ! E`
closure that assigns to its parameter on a path that returns `Err`.
Make a timing dependent golden deterministic (websocket's bye_late
sleeps between the message and the close) rather than relying on a
stress loop.
