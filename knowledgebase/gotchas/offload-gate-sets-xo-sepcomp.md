# An offload gate runs every test with XO_SEPCOMP=0

Status: Observed (2026-11-20, task #507)

Symptom: a cmd/xo test passes on the Mac and fails in an offload gate
(release gate rel-0550 on host 4): `TestStdVariantLineWholeProgram`'s
`--release` case printed `std variant: off (XO_SEPCOMP=0 or
XO_SEPCOMP_DEFAULT=0)` where it wants `not used (--release)`.

Cause: scripts/lead/offload-gate-remote.sh exports `XO_TEST_SEPCOMP=0`
(the whole program, no std variant prebuild), and internal/xotool's
tierSepcomp then sets `XO_SEPCOMP=0` in the test process, which every
xo it starts inherits. xo counts a set `XO_SEPCOMP` (even "") as the
user's choice. A test whose answer depends on `XO_SEPCOMP` being unset
passes only where nothing set it. The test skips under `-short`, so
only a gate running cmd/xo in full reaches it.

Fix: a test that needs it unset removes it from its own process
(`t.Setenv(k, "")` then `os.Unsetenv(k)`, not parallel); the per-test
`setenv` helper cannot unset. Reproduce on the Mac with
`XO_TEST_SEPCOMP=0 go test ./cmd/xo -run NAME`.
