# A big constant table in a std module test slows every -short run

Status: Observed (2026-10-21).

**Symptom.** internal/stdtest's TestStdModules in `go test -short` grew
from 68 s to 270 to 370 s and passed go test's 10 minute default under
pushq's parallel load, failing other agents' pushes.

**Cause.** std/native/llrt/gomath_test.xo held Go's results on 21,460
inputs as one `const` List; compiling it was 143 s alone (254 s under
load), and the modules ran one at a time.

**Fix.** Keep std module tests quick: a sample in the module's
`_test.xo`, the whole generated corpus in
internal/stdtest/testdata/corpus/NAME (public API only), which
TestStdCorpora runs outside -short. The modules now run in parallel
(XO_STDTEST_JOBS). Check a new corpus with `time xo test
--std-module=M` before adding it: over about 10 s, move it.
