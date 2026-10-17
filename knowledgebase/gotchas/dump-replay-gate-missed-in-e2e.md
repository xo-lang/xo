# A dump replay inside a test with other checks escaped decision 0217's gate

Status: Observed (2026-10-14), fixed (2026-10-14).

Symptom: TestSelfhostLLVMEndToEnd (internal/selfhost, full tier) failed
in the Linux arm64 milestone run at main f8ebc9ec with `the IR differs
from the recording: the hash differs from the recorded output's of an
earlier input`, the llvmdump of compiler/lexer, after any edit to the
lexer or std.

Cause: decision 0217 turns recorded dump replays off by default through
skipDumpReplay at the top of each replay test. This test also builds the
component with `xo build --backend=llvm` and compares the binary's output
with the go build's, so it was not one of the "only a replay" tests and
was missed; it runs only in the full tier, which the merge tier never
exercises.

Fix: dumpReplayOn (gogoldens_test.go) is the gate as a bool; the test
compares the IR with the recording only when it is true and keeps its
build and output comparison. An audit (go/ast, every Test function that
reaches recordedPieces through calls in the package) found 17 such
tests on 2026-10-14; all 17 now call skipDumpReplay or dumpReplayOn.
When adding a test that reads testdata/gogoldens, gate it the same way.
