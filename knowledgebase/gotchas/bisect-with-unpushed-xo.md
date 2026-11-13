# A bisect with an xo carrying unpushed work blamed the wrong change

Status: Observed (2026-11-13, twice in one bisect)

Symptom: bisecting the 2-hourly log_error windows/amd64 failure, Lang 3
first blamed std/native/.xo/cbind (a use-c binding cache in the std tree)
and then std/native/llrt/tzrules_test.xo: each toggled the failure. Both
were wrong; the cause was a front end cache bug
(fe-cache-replay-made-interface.md).

Cause: the xo used for the bisect was the worktree's bin/xo, built with an
unpushed compiler change, and it shared the user front end cache. Toggling
a file changed which cache entries were recorded and replayed, so the
failure moved with unrelated files.

Fix: bisect with the failing run's own xo: the cache key its log prints
(`xotool: xo .../checkout/KEY/xo (key KEY, commit C)`) is
`<user cache>/xo/checkout/KEY/xo`; run it on a checkout of C with XO_STD
set, the same environment as the run (hourly.sh exports
XO_INLINE_STRICT=1), and a private XO_BACKEND_CACHE_DIR per trial so no
trial reads another's entries. Check XO_FE_CACHE=off early: if it passes,
the cache is involved, not the toggled file.
