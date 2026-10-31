# TestGoldenRunHeapGuard's http goldens time out under load

Status: Observed (xo-lang-2, 2026-10-31)

## Symptom

`go test ./internal/arm64 -run TestGoldenRunHeapGuard` fails one of the
TLS or http goldens (http_tls, http_tls_keys, http_conn_fault) after
about 30 s ("output differs from want.txt with XO_HEAP_GUARD=1"), on
some runs and not others, while a compiler change is gated. On
2026-10-31 a register allocator change (decision 0448's live-out sets)
failed it in 3 of 8 runs while main passed 8 of 8, which looked like a
miscompile.

## Cause

Not the change: the http_tls_keys binary built by both compilers had
identical __text and __data (6 bytes of UUID and signature differed),
and main's binary passed 20 of 20 runs alone. The subtests run in
parallel, and the first run of a new compiler builds every golden while
the others run: on a loaded Mac (load 28 to 33, the 2-hourly main check
beside it) a TLS handshake under the heap guard starves past the
golden's 30 s limit. Later runs, with the builds cached, pass.

## Fix

Before blaming the change, compare the golden's binary from both
compilers (`otool -t` and `otool -d` hashes), and rerun with
`-parallel 1` (passed 3 of 3). A real fix makes the goldens' deadline
scale with load or runs the http goldens serially in this test.
