# compiler/ssa: constructs used only in tests stay unported until test binaries are compared

Status: Observed (2026-10-11, SSA stage 7)

Symptom: when the test binaries were first compared, one function of
internal/gen/testdata/golden/test_fakes was skipped by the Xo builder
with "`Rand.shuffle`" and a test calling `os.proc.exit` differed by a
missing check: the Go builder lowers both, and the Xo builder had
reported `Rand.shuffle` as unsupported since stage 1.

Cause: a stage of the port is compared on the code the repository's
runs build. Code reached only from test blocks (and `os.proc.exit`'s
check, which exists only in test binaries) was never built before stage
7, so a shortcut there (an `unsupported` for a method the Go builder
lowers through std/native) was never seen.

Fix: both ported (call.xo: rand_shuffle through the native prelude, the
check after `rt.exit` in test binaries). When a later stage widens what
is compared (test binaries, `use c` runs, the stages after the build),
read the skip report (`XO_SELFHOST_KEEP`) for reasons the Go builder
does not report: every skip should match an error of `xo ssadump`.
