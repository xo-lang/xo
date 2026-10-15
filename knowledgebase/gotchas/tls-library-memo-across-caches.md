# The TLS archive's in process memo ignored $XO_TLS_CACHE

Status: Observed (2026-10-10) Obsolete since 2026-10-15: Mbed TLS removed (decision 0202).

Symptom: TestTLSAccel, run after TestTLSLibrary in one `go test`
process, failed to link: `no such file or directory:
.../TestTLSLibrary.../libxotls.a`.

Cause: `internal/build`'s `tlsLibrary` memoized built archives by their
cache key (sources, compiler, flags) only. TestTLSLibrary builds into a
temporary `$XO_TLS_CACHE` that `t.TempDir` removes afterwards; the next
build in the same process with the same key got the removed directory.

Fix: the memo is keyed by the archive's directory (root and key), so a
different `$XO_TLS_CACHE` looks again (2026-10-10).
