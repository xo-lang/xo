# TestGoldenGoParity ran no golden and passed

Status: Observed (2026-11-06)

Symptom: a gate for a Go backend change (compiler/gen, std/rt) printed
`llvm: ok` with TestGoldenGoParity passing in 0.00 s; `-v` shows every
`TestGoldenGoParity/NAME` subtest as SKIP ("not selected
(XO_LLVM_GOLDENS: ...)"). Nothing compared a Go build's output with
want.txt.

Cause: scripts/gate.sh's llvm step picks goldens by diffing each golden's
LLVM IR between the branch's xo and its merge base's
(TestGoldenIRManifest) and exports the names in XO_LLVM_GOLDENS.
TestGoldenGoParity used the same selection. A Go backend change leaves
the LLVM IR unchanged, so the selection was "none" (or only goldens whose
directory the change touched), every parity subtest skipped, and a test
with only skipped subtests passes. This was the gate the Go backend's
change relied on, so it could not see the change.

Fix (2026-11-06): TestGoldenGoParity fails when XO_TEST_GO_PARITY=1 and
-run matched goldens but it built none. For a Go backend change, gate.sh
also sets XO_GO_PARITY_SAMPLE=16, which runs every 16th golden (sorted,
about 38 of 613) beside the LLVM selection. The lead's full tier still
runs every golden on the Go backend. In hand-written gate scripts, grep
`-v` output for indented `    --- PASS` lines, or count them; a parent
`--- PASS` alone proves nothing.
