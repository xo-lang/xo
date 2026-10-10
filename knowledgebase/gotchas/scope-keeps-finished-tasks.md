# A scope kept every task it started until it closed

Status: Observed (2026-10-10, decision 0118)

Symptom: a native HTTPS server under a connection per request grew its
resident memory by about 1 KB per connection (199 MB after 200,000
connections, 293 MB after 315,000); `leaks` found nothing and the
leak check at exit was clean.

Cause: `xo_task_spawn` links every task into its scope's list, which
holds a reference until `xo_scope_close` releases them all. The server's
connection tasks run in one scope (`xo_srv_new`) that closes only when
the server stops, so every finished connection's task and result
stayed. Any long lived scope that starts tasks in a loop does the same.

Fix: a spawn prunes the list when it doubled since the last pruning
(`scope_prune`): tasks whose run and queue entry are over (`gone == 2`)
and whose only reference is the list's go. Peak RSS after 20 s fell from
295 to 223 MB; the rest is the allocator keeping blocks on the freeing
thread's list (see decision 0118, Consequences). The leak check cannot
show this kind of growth: everything is still reachable and is freed at
exit.
