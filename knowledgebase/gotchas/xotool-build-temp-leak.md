# xotool.Build left a temporary directory per build

Status: Observed (2026-10-14, the lead; fixed the same day)

Symptom: $TMPDIR held 19,201 xo-build-* directories (about 70 GB,
9,645 made in one hour of test runs) and 308 xo-golden-want-*; the Mac's
disk fell to 7 GB free.

Cause: `xotool.Build` without `Options.Out` wrote the binary into
`os.MkdirTemp("", "xo-build-")` and returned it as `Result.Dir`, which
no caller removed; internal/llvm's xo1Build did the same with
xo-golden-want-*. Faster, more parallel test runs made more of them.

Fix: both now use `xotool.TempDir`, a directory under one root per test
process (`testtmp.Dir("xo-builds-")`, with an owner file). Every
TestMain that builds calls `xotool.CleanupTemp()` after `m.Run`; a run
killed before that is swept by the next test process (its owner is
gone). TestBuildTempCleaned checks that nothing is left in $TMPDIR. The
TestAttenuationFakes* directories in $TMPDIR (about 420 MB each) date
from 2026-10-13 runs killed before t.TempDir's cleanup; Go's t.TempDir
has no such sweep.
