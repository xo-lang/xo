# TestGoldenThreads cannot link a golden that uses std/time

Status: Observed (2026-10-13, decision 0193 step 5)

Symptom: the new LLVM golden tz_table failed in internal/llvm's
TestGoldenThreads with `Undefined symbols: _xo_tz_data, _xo_tz_find,
...` from clang, while TestGolden passed.

Cause: TestGoldenThreads compiles the generated IR with the runtime by
hand (`-DXO_RT_TEST_THREADS=4` and IOFlags), not through internal/build,
so it has neither -DXO_TIME nor the program's zone data object
(xo_tzdata.c) that a std/time program links.

Fix: the test skips a program whose IR refers to `@xo_tz_` ("std/time
needs the program's zone data"); TestGolden, TestGoldenSplit,
TestGoldenShared, and arm64's TestGoldenRun cover tz_table. Next step
if std/time should run under the thread stress too: write the zone data
with internal/build's tzSource and add -DXO_TIME in the test.
