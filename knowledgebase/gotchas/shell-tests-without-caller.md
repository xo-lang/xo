# A shell test nothing runs goes stale unnoticed

Status: Observed 2026-11-21 (task #588)

## Symptom

`scripts/lead/pushq_prelane_test.sh` printed
`FAIL a test file's pub and a private line: no stdcov: got [corpus], want [corpus fast stdchecks stdgen]`
when run by hand. It had been failing since 8ee6e3324 (#579), and no gate or tier had reported it.

## Cause

56 of the `scripts/lead/*_test.sh` files had no caller: no internal/repotest Go test ran them, and
no gate, push or tier did either. #579 made a push that adds a std test file need `stdwasmnew` in
PUSHQ_RAN. The test's `push()` helper did not name it, so pushq refused that case (rc 4), and the
test checked only which pre-lane checks ran, not that the push landed. The scripts are written and
run once by their author, and then only by hand.

## Fix

Every `*_test.sh` now has a caller: a Go test, or a line in `scripts/lead/shelltests.txt`, which
internal/repotest TestLeadShell runs. TestLeadShellListed fails for a new test with neither, and for
an entry whose inputs (the scripts it names, followed through `$here`-style references) are not under
its listed prefixes. pushq's fast checks skip a subtest when a push changes none of its inputs. When
you add a shell test, run `go test -run TestLeadShellListed ./internal/repotest` and paste the line it
prints. A test that checks what ran should also check that the push landed.

TestLeadShell runs on darwin only: on linux/arm64 host 3 (2026-11-21) 10 of the 55 failed for Mac
assumptions alone (BSD `date -v`, `sed -i ''`, plutil, expect, a git identity). offload_test.sh
talks to a real host and ran past the 10 minute timeout on the Mac, so it is a `manual` line.

Under pushq's fast checks the first push failed gate_failfast and gate_output with XOBUILD-FAIL:
pushq-env.sh's `env -i` dropped HEAVY_JOB, so xotool.Unguarded refused gate.sh's xobuild on the Mac
(exit 64) though heavy.sh ran it. pushq-env.sh now keeps HEAVY_JOB (pushq_env_test.sh checks it).
Reproduce a fast check as pushq runs it: `heavy.sh NAME scripts/lead/pushq-env.sh go test ...`.
