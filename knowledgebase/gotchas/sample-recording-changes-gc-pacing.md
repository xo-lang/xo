# Recording samples in the server doubled its throughput (GC pacing)

Status: Observed (2026-10-08, bench/perf/http task 01, 32 workers)

Symptom: with `XO_LOCKSTATS` on, the Xo server (no arenas) did 115k to
127k req/s, while `bench/perf/http/run.sh` measured the same build at 61k.
Instrumentation made it faster.

Cause: the lock statistics keep every sample in memory, about 50 MB by
the end of a run. The Go GC paces on the live heap (`GOGC=100`): the
uninstrumented server has a live heap of a few MB, so it collects every
few MB of allocation (173 GCs per 10k requests, 15 percent GC CPU);
with 50 MB of samples alive it collects a tenth as often. The server is
GC bound, so throughput follows. A single run with `GOGC=800` (no
instrumentation) gave 137k req/s for Xo and 136k for Xo `--arenas`.

Fix: never compare throughput from an instrumented run with an
uninstrumented one; take req/s from `run.sh` and hold and wait times
from the instrumented runs (all configs instrumented alike, Go too).
Lock waits measured with lock stats on are lower than in a plain run
(less GC, fewer descheduled holders); compare them only with each other.
