# internal/llvm's diff tests built the arm64 backend on x86 hosts

Status: Observed (2026-10-14), fixed (2026-10-14).

Symptom: the linux/amd64 milestone run at main f8ebc9ec (c7i.8xlarge)
failed TestCABIDiff, TestDigestDiff, TestIntsDiff, TestJSONTextDiff,
TestNumDiff, TestScanDiff, and TestTZDiff (embedded, system, zoneinfo),
each only in its `arm64` subtest: `prog-linux.0.s: Error: no such
instruction: stp x29,x30,[sp,...`. The llvm subtests passed. TestTZDiff's
"6 unknown zones" line is its log of the Go reference's output, not a
difference.

Cause: these tests loop over `{arm64, llvm}` with no host check. `xo
build --backend=arm64` on an x86 host writes arm64 assembly and hands it
to the host's cc, whose x86 assembler rejects it. Other tests in the
package (gen_native, threads, signal, ...) add arm64 only on an arm64
host; these seven never ran on an x86 host before the amd64 milestone.

Fix: xotool.Arm64Host and xotool.Arm64Skip are the one host rule
(darwin/arm64 or linux/arm64); internal/llvm's buildRun skips an arm64
build elsewhere with that reason, so every diff test's arm64 subtest
skips, and internal/selfhost's nativeSkip uses the same rule.
