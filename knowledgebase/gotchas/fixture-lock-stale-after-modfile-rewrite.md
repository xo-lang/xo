# A fixture's xo.lock goes stale when its cached modules are rewritten

Status: Observed (2026-10-12, decision 0177).

**Symptom.** The 4 runs of internal/selfhost/testdata/modules/app
(core, ok, web, web/api), meant to exercise deny rules, effect
ceilings, and an accepted module with dependencies, were all rejected
with XO1304 (seven per run) by both checkers. The differential test
counted them as matches, so nothing failed.

**Cause.** Decision 0155 rewrote every xo.mod to `edition preview`,
including the modules of the fixture module cache
(internal/selfhost/testdata/modules/cache). Their content hashes
changed; app/xo.lock still held the old ones, and XO1304 stops the
build list before the rules the runs test.

**Fix.** After any mass edit of files under a fixture module cache,
re-run `xo mod tidy` in each fixture that requires those modules (with
`XO_MODCACHE` pointing at the fixture cache and `XO_OFFLINE=1`; delete
the stale lines first). When a test's matches suddenly include a new
rejection code on fixtures meant to be accepted, read which runs it
comes from (`XO_SELFHOST_KEEP`) before trusting the match count.
