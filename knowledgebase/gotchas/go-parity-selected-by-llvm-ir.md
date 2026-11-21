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

Again (2026-11-20, task #552): #540's std/rt/list.go fault text change
gated with 0 of 859 goldens selected, and the every-16th sample did not
include any of the four `bytesbuilder_put*` goldens whose want.txt held
the new text. Main failed GoParity on them until the fix. Fix: for a Go
backend change, gate.sh's `rt_string_goldens` also runs
TestGoldenGoParity on every golden whose want.txt or want_go.txt holds a
string literal piece (8 or more characters, split at format verbs) that
the std/rt/*.go diff adds or removes (scripts/golden-rt-strings.py, at
most 64 goldens; the last 15 std/rt commits select 0 to 29).
