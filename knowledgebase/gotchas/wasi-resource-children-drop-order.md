# A WASI resource dropped while its children live traps

Status: Observed (2026-11-03, decision 0447 R2a; golden http_stream_body
under TestWasmCommandGoldens, 2 of 3 runs)

Symptom: a wasip2/wasm command ends with exit 1 after printing all its
output: `failed to invoke run function ... sk_lis_free ... rc_defer ...
resource has children`. Intermittent: it depends on whether a task was
waiting on the resource when it was freed.

Cause: in the component model a pollable from `subscribe` (and an
input or output stream) is a child of the resource it came from, and
dropping a parent with a live child traps. rt9.xo's `$sk_lis_free` (the
free entry of a listener the program never closed) dropped the socket
while a canceled accept's pollable was still in the wait list
(`$wq`): http.Server.serve's accept task is canceled at a stop, and
nothing removed its wait. Listener.close ran `$sk_unwait` first, the
free entry did not.

Fix: every path that drops a socket drops its waits first (`$sk_unwait`
before `$sk_drop`), and its streams before the socket
(`$sk_conn_shut`'s order: output stream, input stream, socket).
Check a new wasi resource free entry against the close path; a
golden run once is not enough, run it several times with the harness's
flags (wasm_cmd_test.go).
