# `xo serve` does not notice a new xo.work above the checked directories

Status: Anticipated (2026-10-09)

Symptom: with `xo serve` running, creating an `xo.work` in a parent of
the checked module (or deleting `vendor/`) can leave `xo check` answering
from the cached result until some source file changes.

Cause: the serve cache stamps files that exist: every .xo, xo.mod,
xo.lock, and xo.work under the checked directories, plus the xo.mod,
xo.lock, vendor/modules.txt, and active xo.work of every loaded module. A
file that did not exist when the result was cached has no stamp, unless
it appears under the checked directories (the walk sees it).

Fix: `xo serve stop`, or `XO_NO_SERVE=1`, or pass `--workspace=off` /
`--offline` (any module flag makes `xo check` run in process). Next step:
stamp the absence of xo.work in each ancestor directory.
