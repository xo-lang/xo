# 0217: Verify the compiler by behavior, not by recorded internal dumps

Status: Accepted (owner, 2026-10-13: "I don't want another 25-40
recording. That's wasteful."); step 1 implemented, the rest is the
`xo dev` harness work (decision 0209 phase 2)

## Context

After the Go compiler was deleted (decision 0215), internal/selfhost
replays recordings of the compiler's internal output (typedump, ssadump,
asmdump, llvmdump, gendump, defhash, checker mutations) for every input
and requires the same hashes. The recordings proved the Xo compiler
matched the Go compiler during the port. With the Go compiler gone they
mostly freeze today's output: an intended change (decision 0212 stage 2's
pool.xo changes every program's runtime) needs a re-recording that took
25 to 40 minutes on the Mac (2,452 s and 1,449 s; 1,413 to 2,110 s on
EC2, bench/results/xo-dev-harness-2026-10-13.md), twice on 2026-10-13.
Go verifies its compiler with program output tests (test/ `// run`),
error checks (`// errorcheck` with `// ERROR` comments), assembly pattern
tests (test/codegen), unit tests, and the bootstrap; it keeps no
recordings of whole internal dumps.

## Decision

1. Now: the back end and mutation dump replays (TestSelfhostSSA,
   SSAMutations, Arm64, Arm64Mutations, LLVM, LLVMMutations, Gen,
   GenMutations, Defhash, CheckerMutations, CheckerModuleMutations,
   CheckerHatches) are skipped by default (`skipDumpReplay`);
   `XO_SELFHOST_DUMPS=all` runs them, and record runs still record. The
   front end replays (lexer, parser, format, checker) stay on as a cheap
   tripwire: their output changes only with the language.
2. The compiler is verified by, in order of weight:
   - program output goldens on every backend (want.txt, exit status,
     stdout, stderr), including the cross-backend comparison;
   - the self rebuild fixed point for compiler and runtime changes;
   - error check tests: expected diagnostics as comments in the source;
   - codegen pattern tests for optimizations that matter: per function
     assertions on the emitted assembly or IR, not whole files;
   - random programs (progen) compared across backends;
   - Xo unit tests (`xo test`) next to the code (inline test blocks for
     small functions, `_test.xo` for suites, decision 0209's convention).
3. The `xo dev` harness (decision 0209 phase 2) adds the error check and
   codegen pattern runners and measures each step before and after.
   Recordings that no test uses are then deleted from
   internal/selfhost/testdata/gogoldens.

## Consequences

- An intended compiler change needs no re-recording; its proof is the
  goldens, the fixed point, and targeted tests.
- An unintended change in generated code that still runs correctly is no
  longer caught by default; codegen pattern tests cover the cases that
  matter, and XO_SELFHOST_DUMPS=all remains for a deliberate audit.
- Expected cost per change, to be measured by the harness: about 3 to 5
  minutes plus the fixed point (about 17 minutes on the Mac until builds
  are parallel).

## Alternatives

- Keep recording everything and make recording faster: still a full run
  per intended change, and a reviewer cannot read hash diffs.
- Drop the front end replays too: they are cheap and catch language
  regressions; kept until error check tests cover the same ground.
