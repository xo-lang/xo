# A native prelude fix turned TestGoldenGoParity red

Status: Observed (2026-11-21)

Symptom: after f15e953544 (task #597) kept a sub Fs's trailing
separator in std/native/prelude.xo, the lead's tier on main failed
TestGoldenGoParity/fs_sub_trailing_slash 4/4: the Go backend printed
"sub read a.txt/: found" where want.txt says "not found". The push had
gated on the native backends only, where the new golden passed.

Cause: the Go backend does not run std/native/prelude.xo. Its prelude is
std/rt (Go), so `Fs` there is std/rt/caps.go's realFs, with its own
resolve. A behavior fix in the native prelude leaves the Go backend
unchanged, and a new golden that shows the fix fails there.

Fix: port the change to std/rt in the same push (926beac0a0, task #629:
realFs.resolve keeps the trailing separator) and run the new golden on
the Go backend before pushing:
`XO_TEST_GO_PARITY=1 go test -run 'TestGoldenGoParity/^NAME$' -v ./internal/llvm/`
(without XO_TEST_GO_PARITY=1 the test skips and passes; see
go-parity-selected-by-llvm-ir.md).
