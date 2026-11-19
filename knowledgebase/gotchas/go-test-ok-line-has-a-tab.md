# `go test`'s "ok" line has a tab, not only spaces

Status: Observed (2026-11-19, task #436)

**Symptom.** A script that decides pass or fail from `go test` output
with `grep -E '^ok +github.com/...'` never matches a passing package.
golden-freecheck.sh (task #350) would have reported a run in which every
golden passed as "ended before a golden result"; its runs so far had all
trapped a golden, so nobody saw it. Its test did not catch it either: the
check `[ A ] && [ B ] || [ C ] && [ D ]` is `((A && B) || C) && D` in
bash, so a pass case's unexpected trap line slipped through.

**Cause.** `go test` writes `ok`, two spaces, then a tab before the
package path (`ok  \tgithub.com/xo-lang/xo/internal/arm64\t3600.1s`).
` +` matches spaces only.

**Fix.** Match `^ok[[:space:]]+PKG` (and `[[:space:]]` after the package,
not `\b`, which BSD and GNU grep treat differently). Group compound test
conditions in shell tests with `{ ...; }`. Fixture logs in tests use the
real tab, so the test reads what `go test` writes.
