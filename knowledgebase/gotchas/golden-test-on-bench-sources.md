# A golden test that compiled the benchmarks went stale with a benchmark edit

Status: Observed (2026-11-22, task #698, the 0.55.1 candidate's tier 928 on the Mac).

Symptom: internal/arm64 TestGoldenAsm failed on all four goldens (fib and sieve, darwin and linux) at
the release candidate, with a large diff: fib's frame went from 48 to 80 bytes, three more registers
saved, calls of process.args and prelude.texts in the first function. It read as a code generation
regression two commits before a release.

Cause: the test compiled bench/perf/llvm/xo/fib and sieve. 98335486d8 (task #686) gave every program
there a repeat count and a `work` function so that timed runs last long enough; main grew, and the
goldens of the old main were left as they were. The compiler had not changed: the `// func fib` block
was the golden's on both systems once label numbers were set aside. Nothing ran the test between the
commit and the candidate: the push asked for no goldenasm step for a change under bench/, and the
last continuous tier on main was at a commit three minutes older.

Fix: TestGoldenAsm compiles its own copies (internal/arm64/testdata/asm/fib and sieve, the sources as
of v0.55.0), and the four goldens pass unchanged. internal/repotest TestGoldenAsmOwnsInputs fails if
the test reads a program under bench/ again.

How to avoid it: a test that compares output with a golden file owns its input. A benchmark, an
example or a doc sample is edited for its own reasons by people who do not know a golden hangs on it.
Tests that only run such a program and check what it prints (internal/arm64 TestBench) may share it.
