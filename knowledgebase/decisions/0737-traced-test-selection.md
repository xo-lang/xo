# 0737: traced test selection for program and compiler tests

Status: Proposed (owner, 2026-11-21: "yes" to claiming the ADR and filing
steps 1 and 2; the design below is a stub, to be filled after step 1's
measurement). Builds on 0125 (definition hashes), 0267 and 0701 (test
selection by names), 0439 (test result cache), 0350 (deterministic runs),
0382 (per function passes) and 0657 (golden selection by built binaries).

## Context

A test block's `os` is a TestOs (MemFs, virtual clock, seeded Rand, FakeNet;
core 9.2), Xo has no global variables, and the native scheduler is seeded
(0350). So a test without `uses` has an outcome that is a function of the
code it executes and its seed: if no definition a test executed changed, its
outcome cannot change. In Go or Java this rule is unsound (a test may read
the clock, environment or files); in Xo it holds.

Today's selection guesses from outside: `--watch` and `--since` match names
in the source text (0267, 0701), the result cache replays only an unchanged
run (0439), and 0657 compares whole golden binaries.

Ceiling (tier 920, offload host log lead-tier920-off-cmd.log, timed steps
only): of 9,439 s, the deterministic steps (llvm 3,392, selfhost 2,240,
stdwasm 564, gen 446) are 6,642 s (70%); cmd 1,534 and stdcov 1,132 (28%)
are partly selectable; fuzz 131 (1.4%) is not. At 3 tiers a day that is
5.5 host hours a day selectable exactly, 2.2 partly. These are ceilings:
how many binaries change between consecutive tiers is not yet measured.

Step 1, first counts (xw-1, 2026-11-22, task #525; host 2, linux/arm64,
arm64 backend, every golden of test/golden built by both xos and compared
with scripts/identity; ~/xo-lead/notes/xw-1/525-build-breakdown.md):

- 7e1c1ce2e0 to 1df67288a1 (2 commits, one closure file, 14+/1-): 432 of
  432 binaries identical (the best case).
- 0f2ad3121a (compiler/ssa, generic instance thunks, 22+/16-) against its
  parent: 283 of 430 identical (66%), 147 differing, 0 failed.

So a pass change leaves about two thirds of the golden binaries identical.
Builds are path independent (three goldens built from two directories gave
identical binaries on the native and LLVM backends, 2026-11-22), so a
tier's new checkout path does not change a binary.

## Decision (proposed, in steps)

1. Measure: for the commit pairs of the last 10 tiers, build every golden
   (arm64, llvm, amd64) and every std module test binary with both xos and
   count the changed binaries. This bounds what steps 2 to 4 save.
2. Binary keyed tier steps: extend 0657's selection from the gate to the
   tier; a golden or std test binary identical to the last passing tier's is
   not rerun. The llvm step's fixed floor (3,056 s with 0 of 717 goldens
   selected, gotchas/gate-fixed-floor.md) is removed in the same step.

   As built (task #610, 2026-11-22): the key is per golden subtest, not
   per step (a step of the continuous tier fails somewhere most runs, so a
   whole-step rule would skip nothing). internal/xotool goldenpass.go keys
   llvm TestGolden and arm64 TestGoldenRun on the binary's SHA-256 with
   the build-info id masked (0657's rule), every file of the golden's
   directory but its .xo sources (want.txt, args, stdin, data), the host's
   OS and architecture, the test name and its run settings (leak check,
   free check). The continuous tier (ctier.sh) records and skips
   (`XO_GATE_GOLDEN_PASS_DIR=/work/golden-pass`, `XO_GATE_GOLDEN_PASS_SKIP=1`);
   the 8-hourly tier records and never skips, so a full unselected tier
   still runs on its cadence. A failing run removes the golden's key, a
   fail and a pass of one golden in one merge remove it, so only a binary
   identical to the last passing run's is skipped. The skip saves the run;
   the build still happens (the key is the binary).

   Std module test binaries are not keyed: no step of the continuous tier
   runs them (stdcov and stdwasm run only in the 8-hourly tier, which never
   skips), so a key would save nothing until one does.
3. Execution traces: `XO_TEST_TRACE` records the defhashes (and the machine
   code hash of each function) a test executed; selection reruns a test only
   when its recorded set holds a changed hash. The compiler gets the same
   trace while it compiles each golden function, so a compiler change maps
   to the golden functions whose compile touched it.
4. Pass replay: each compiler pass's recorded inputs (the 0382 cache
   entries) form a corpus; an edited pass reruns on them, and identical
   outputs prove no golden can change. Differences report as a behavior
   diff and continue only those functions downstream.

Never selected away: tests with `uses` (real capabilities), fuzz
exploration, stress and soaks, and a full tier run at least once a day that
checks the selection missed nothing.

## Consequences

Step 2, measured (xw-1, 2026-11-22, host 2, linux/arm64, 568f2fb87e built
with XO_BACKEND_CACHE=off, offload job gp610; log
~/xo-lead/agents/xw1gp610m.59225-cmd.log): two rounds of llvm TestGolden
and arm64 TestGoldenRun over 866 goldens, the second skipping.

| step | round 1 (records) | round 2 (skips) |
|---|---|---|
| llvm TestGolden | 924 s, 866 pass | 907 s, 865 skipped, 1 pass |
| arm64 TestGoldenRun | 220 s, 866 pass | 198 s, 865 skipped, 1 pass |

The skip is exact (one golden's key differed between the rounds, its binary
or a file in its directory, not yet traced, and it ran), but it saves little: the 866 runs sum to 96 to 97 s
(conc_scope_bg_outlive alone 21 s), so the steps are their builds. The
compiler-change floor of the per-push gate (HEAD's IR of every golden,
1,132 to 1,703 s in 17 gates of 2026-11-20 to 11-22,
gotchas/gate-fixed-floor.md) is not one binary keys can remove: it moves
to step 3, which must avoid the builds, not the runs.

## Alternatives

To be filled after step 1: names-only selection (0701, today), whole binary
keying only (0657), translation validation of changed functions.
