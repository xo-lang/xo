# The Ed25519 curve constants were initialized behind a plain flag

Status: Observed (2026-10-12, ThreadSanitizer on Linux), fixed in
std/llrt/xo_crypto.c (cr_ed_init).

Symptom: TestGoldenThreads/native_ed25519 reported a ThreadSanitizer data
race on the global `cr_ed_ready` between cr_ed_init on two worker
threads (xo_crypto.c:1363 read, 1383 write).

Cause: cr_ed_init computed d, 2d, sqrt(-1), and the base point on first
use and then set an int flag. Two tasks on two workers could both see
the flag clear and write the constants while the other read them; on a
weakly ordered CPU a reader could also see the flag before the
constants.

Fix: pthread_once (cr_ed_once, cr_ed_init_once). Any lazily computed
global table in the C runtime needs pthread_once or an atomic
acquire/release flag; a plain flag is a race even when every writer
writes the same values.
