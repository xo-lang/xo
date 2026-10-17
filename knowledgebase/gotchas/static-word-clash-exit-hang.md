# A static word outside the module's range corrupts another module's state

Status: Observed (2026-10-17, decision 0251's branch)

## Symptom

HTTP and TLS programs (goldens http_tls, http_tls_keys, http_conn_fault)
hung at exit, on every backend and with --race: main in pthread_join, the
workers parked in pool.go_idle, nothing told them to stop. Other programs
were fine.

## Cause

ctx.xo stored a new registry pointer in `rt.static_word(18)`. ctx.xo's
range in std/native/rt.xo's table is 16 to 17; word 18 is tzhost.xo's
lock (the kept zones' lock). The pointer made the lock look held, and
time zone work at the end of those programs waited forever.

## Fix

Check std/native/rt.xo's table (one contiguous range per file, decision
0237) before using a static word; extend a file's range only into free
words and update the table, or keep the value in a record the module
already owns (0251 put the registry in ctx.xo's services record).
TestStaticWords (internal/repotest, in -short) fails on a word outside
its file's range: run -short first in a gate, before the long goldens.
