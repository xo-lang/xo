# A gate step's fixed floor: selecting fewer tests did not make it faster

Status: Observed (2026-11-14, lead; owner's test-time plan)

## Symptom

The per-push gate's llvm step took 35 to 68 minutes whether it selected
every golden program, a few, or none: Lang 3's feslot2 gate selected 0 of
717 goldens and its llvm step took 3,056 s; civ2 selected 3 and took about
34 minutes. Over two days the step averaged 46 minutes across 44 gates.

## Cause

Two costs that did not depend on the selection:

1. The selection itself. It compares each golden's LLVM IR at the merge
   base and at HEAD; the base's manifest is cached by commit, but main
   moves between almost every gate, so nearly every gate built the base's
   xo and every golden's IR again before choosing.
2. The package's other tests. About 130 top-level tests (TLS, x509 and
   crypto differentials, the race tables, scheduler and preemption tests)
   ran in every gate, 853 s of them one after another before the parallel
   phase (civ2's llvm.json, 2026-11-12).

The golden selection (2026-10-22) had cut the part that scaled with the
change and left the part that did not.

## Fix

- The base's manifest may be an ancestor's within 30 commits (a superset
  of the base's selection), so a gate makes only HEAD's.
- scripts/gate-areas.txt scopes the slow tests to the paths that can break
  them; the full tier still runs every test.
- Every gate records its step and test times, and scripts/lead/gatetime.sh
  checks a day of them once a day, including this floor (an llvm step over
  900 s with at most 5 goldens selected).

## How to avoid

When a step gains a selection, measure the step with an empty selection:
what remains is the floor, and it is what every unrelated change pays.
toolchain.md "Gate test areas"; bench/results/gate-time-2026-11-14.md.
