# A new field of a runtime object left uninitialized at an older allocation site

Status: Observed (2026-10-09, decision 0083)

Symptom: after the test world added `mem` to `xo_listener` (std/llrt),
every golden that listens on a real socket (net_echo, http_*) failed
with exit -1 or, under UBSan (TestGoldenASan), "member access within
misaligned address 0xbebebebebebebebe" in `tw_listener_free`.

Cause: `xo_alloc` does not zero memory (0xbe is the pattern of a fresh
block under the sanitizers), and the socket path of `xo_net_listen` set
the fields it knew about; `listener_free` then read the garbage `mem` as
an in memory listener.

Fix: set the field at every allocation site (`l->mem = NULL`). When
adding a field to a runtime struct, grep every `xo_alloc(sizeof(<type>))`
of it, and run the golden suites with sanitizers
(`go test ./internal/llvm -run TestGoldenASan`), not only `-short`.
