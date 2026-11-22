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

## What is left (2026-11-22, xw-1, task #610)

17 gates with an llvm step, 2026-11-20 to 11-22 (notes/gate-times):
a change whose `xo build --list-inputs` scope is small selects in 26 s
(t540: 0 goldens, step 743 s, the package's other tests 721 s beside it).
A change to the compiler has scope "all", so the gate builds HEAD's IR of
every golden before it can select: 1,132 s for 1 golden (l454), 1,703 s
for 0 (l450). That build is the question the selection asks (could this
golden's code change?), so binary keys cannot remove it; execution traces
(decision 0737 step 3) are the step that answers it without compiling
every golden. The tier never selects (XO_GATE_LLVM=all), so it has no
floor; its cost is builds and runs, which step 2's pass keys cut.

## A second hidden cost: parents of parallel subtests

After the fix the arm64 step still took about 29 minutes while its
top-level tests of a second or more summed to 970 s. go test -json's
Elapsed for a parent leaves out its parallel subtests, so the slowest
tests did not show in any per-test list: in tier 915's arm64.json,
TestDebugInfoSameCode reported a few seconds and its subtests summed to
5,290 s (TestNoLeafWithPads 2,951 s, TestIfaceLexMatchesParse 2,100 s).
gate.sh's times.tsv now records each parent's subtest sum (kind
`subtests`, the largest per depth) and gatetime.sh flags an unscoped one
of 600 s or more.

## How to avoid

When a step gains a selection, measure the step with an empty selection:
what remains is the floor, and it is what every unrelated change pays.
Rank a package's tests by their subtests' summed time, not by the parent's
Elapsed.
toolchain.md "Gate test areas"; bench/results/gate-time-2026-11-14.md.
