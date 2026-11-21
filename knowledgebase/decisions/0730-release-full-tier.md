# 0730: A release candidate passes the full tier before the tag

Status: Accepted (owner, 2026-11-20)

## Context

Pushes gate on the minimum (the std Proven push rule, 2026-11-15: selfbuild,
seedbuild and the one or two tests that prove the change), and the full
suites run on main on a schedule. Earlier releases were tagged without one
full run of every suite on the exact candidate, and bugs were found after
the release. For 0.55.0 the lead ran the full tier (tier.sh: every golden
suite on every backend, the Mac, linux/arm64, linux/amd64 and Windows
steps, sdk-stress, fuzz, stdcov) on each candidate. Tier 918 on candidate 4
(fcb74a950) found real user bugs before the tag, among them #558: `xo test
--race` of a program with `use c` C sources failed wherever a race runtime
is installed, likely shipped in 0.51.1 (unverified). Notes:
~/xo-lead/notes/lead/tier918-triage.md (untracked).

## Decision

1. Every release candidate (the exact commit to be tagged, version commit
   included) passes the full tier before the owner tags it.
2. Each failing step is either fixed (a fix with a failing-first test,
   then a new candidate) or listed as a Known issue in the release notes
   with the owner's OK. Infrastructure failures (a host missing a tool, a
   memory stop) are fixed and the step rerun; they are never waived.
3. A step with no result (stopped, skipped, no slot) counts as failing.
4. A fix taken into the release cuts a new candidate, and the full tier
   runs again on it. A docs-only change after the tier needs no rerun.
5. The lead records each candidate, its tier run and each failure's
   verdict in the release's OWNER-TAG.md; the owner tags only when it says
   READY.

## Consequences

- A release takes one full tier per candidate (hours, on the offload
  hosts and the Mac) and usually more than one candidate.
- The daily full tier on main (xw-1 task #563) keeps main close to green,
  so a release tier mostly confirms a known state.
- Bugs found after each release are counted in xw (task #564), so this
  rule's effect is measured against earlier releases.
- `scripts/lead/tier-check.py N` (task #567) lists a finished tier's
  steps as PASS, FAIL or NO-RESULT (item 3) and exits 0 only when every
  step passes: the readiness line for OWNER-TAG.md.

## Alternatives

- Tag on the scheduled main checks alone: what earlier releases did; it
  missed the bugs above.
- A sample tier per candidate: faster, but tier 918's failures were spread
  over backends and hosts a sample would not have covered.
