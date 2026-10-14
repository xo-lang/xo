# The harness self test timed out building xo inside an adapter run

Status: Observed (2026-10-14), fixed (2026-10-14).

Symptom: internal/benchharness TestHarness failed in the linux/amd64
milestone run at main f8ebc9ec: test_first_try_after_xo_fix raised
`subprocess.TimeoutExpired: ... run_openai_compatible.py ... timed out
after 600 seconds`. The other 11 Python tests passed, and the linux/arm64
run of the same commit passed.

Cause: bench/harness/common.py xo_bin uses $XO_BIN, else runs `go run
./scripts/xobuild` to make the checkout's bin/xo current. The first
test that needs xo is an adapter test, which runs the adapter as a
subprocess with a 600 s timeout, so a cold xo build (or the wait for
another package's build under xotool's lock) counted against it. The
build took 426 s on r8g.8xlarge (arm64 backend), under the limit; on
c7i.8xlarge xo builds with the LLVM backend and took longer.

Fix: TestHarness builds xo first (xotool.Path) and passes it as $XO_BIN
unless XO_BIN is set, so the Python tests time only their own work.
