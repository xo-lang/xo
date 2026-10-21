# `go test -run` that matches nothing, or only part of its filter, passes

Status: Observed (2026-10-21: Xo Lang 3's and Xo lang 5's gates)

Symptom: a gate's `go test -run X ./pkg` step reports ok, but the test it
was meant to prove never ran. Two forms:

1. The filter matches no test: `go test` prints `testing: warning: no
   tests to run` (and `ok ... [no tests to run]`) and exits 0. Xo Lang 3
   filtered internal/llvm's goldens by the golden's name, but they are
   subtests: `TestGolden/NAME` in internal/llvm, `TestGoldenRun/NAME` in
   internal/arm64.
2. An alternation where only some names exist (`-run 'TestA|TestB'` with
   TestB misspelled): TestA runs, so there is no "no tests to run"
   warning, and the missing TestB goes unnoticed (Xo lang 5).

Cause: `-run` is a regular expression filter, not a list of tests that
must exist; go test treats matching nothing, or fewer tests, as success.

Fix: in a gate, fail a step whose output says `no tests to run`, and for
named tests run `-v` and require one `--- PASS: NAME` line per name in the
filter (anchor it: `-run '^(TestA|TestB)$'`, and spell subtests in full,
`TestGolden/NAME`). scripts/lead/gatelib.sh does both (`gotests PKG
'TestA|TestB'`, `no_tests_check FILE`); source it from a gate script.
