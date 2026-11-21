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

## Decision (proposed, in steps)

1. Measure: for the commit pairs of the last 10 tiers, build every golden
   (arm64, llvm, amd64) and every std module test binary with both xos and
   count the changed binaries. This bounds what steps 2 to 4 save.
2. Binary keyed tier steps: extend 0657's selection from the gate to the
   tier; a golden or std test binary identical to the last passing tier's is
   not rerun. The llvm step's fixed floor (3,056 s with 0 of 717 goldens
   selected, gotchas/gate-fixed-floor.md) is removed in the same step.
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

To be filled after step 1.

## Alternatives

To be filled after step 1: names-only selection (0701, today), whole binary
keying only (0657), translation validation of changed functions.
