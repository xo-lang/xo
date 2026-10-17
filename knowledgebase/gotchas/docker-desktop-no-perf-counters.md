# perf stat counts nothing in Docker Desktop's Linux VM on the Mac

Status: Observed (2026-10-17, the quiet HTTP measurement, Xo Lang 3)

Symptom: in scripts/docker-linux.sh's arm64 Debian image (run as root),
`apt-get install linux-perf` succeeds and
`/proc/sys/kernel/perf_event_paranoid` is 2, but `perf stat -e
instructions:u -x, true` prints nothing: no count, no error line.

Cause: Docker Desktop's Linux VM on Apple silicon exposes no hardware
performance counters to the guest, so the instructions event is not
available (and Debian's `perf` wrapper wants the running kernel's
version as well).

Fix: count with valgrind instead: `apt-get install valgrind` (3.24 in
trixie) and `valgrind --tool=callgrind`; its `I refs` is the same run to
run for a single threaded workload and agrees within about 0.5% for the
HTTP server (bench/perf/http/callgrind.sh). The CI measure suite's `perf
stat` path needs a real Linux host (the EC2 runners).
