# A parallel top level test does not overlap the serial ones

Status: Observed (2026-10-14, bench/results/short-tests-2026-10-14.md)

Symptom: marking one slow test `t.Parallel()` did not shorten its
package: internal/xotool's TestArchiveTree still ran after
TestSeedPackHello, and internal/llvm's parallel runtime tests all started
7.4 s into the run, after TestRealtimeNative and the signal tests.

Cause: `go test` pauses every parallel top level test until all serial
top level tests have finished, then runs the parallel ones together
(up to -parallel). A serial test's own parallel subtests run inside it,
before the next top level test starts.

Fix: make the slow tests parallel together, or the serial chain the
short part: a package's time is the sum of its serial tests plus the
longest stretch of the parallel ones. Under a shared, CPU bound Mac
(load average 20 to 60) all packages of the -short run then finish in
the same window, and only less CPU work makes the run shorter.
