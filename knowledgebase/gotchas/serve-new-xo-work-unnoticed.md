# `xo serve` did not notice a new xo.work above the checked directories

Status: Anticipated (2026-10-09), fixed the same day; kept for the cause.

Symptom: with `xo serve` running, creating an `xo.work` in a parent of
the checked module could leave `xo check` answering from the cached
result until some source file changed.

Cause: the serve cache stamps files that exist: every .xo, xo.mod,
xo.lock, and xo.work under the checked directories, plus the xo.mod,
xo.lock, vendor/modules.txt, and active xo.work of every loaded module. A
file that did not exist when the result was cached has no stamp, unless
it appears under the checked directories (the walk sees it). An xo.work
that existed but did not use the module was not stamped either, so
adding the module to it went unnoticed.

Fix: the cache entry records, per module root, which xo.work a load
would consult (`mod.NearestWork`, used or not) and stamps it; `fresh()`
recomputes the nearest xo.work and treats a different answer as stale
(`internal/serve/serve.go`, `TestCacheNoticesNewWorkspace`). Still
unstamped: a `vendor/modules.txt` created in a module outside the checked
directories; `xo serve stop` or `XO_NO_SERVE=1` covers that.
