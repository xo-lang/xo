# Two threads moved the native virtual clock for one idle instant

Status: Observed (2026-10-12, EC2 c8g.4xlarge, Linux arm64, 12 test
loops in parallel; after the fix in mem-listener-backlog-after-serve.md)

Symptom: native test binaries (`xo test --no-gc`, arm64 and llvm) in
internal/gen/testdata/golden/test_shutdown failed about 1 run in 40 on
a loaded machine: a task's `os.clock.sleep(1500ms)` returned with the
clock at 2.5s instead of 1.5s ("a request after the stop fails at once
while the listener lives on": `left: (true, 2.5s)`), and once main ran
until its 30s grace period (`os.clock.since(start)` 31s instead of
2.5s). Never on the Go backend, rarely on a quiet machine.

Cause: std/llrt's test world moves the virtual clock when the deadlock
check (dl_check_locked, under pool.mu) finds every context blocked: it
set the flag tw.vpending, and the thread that set it called tw_kick
after releasing pool.mu, which took the flag and fired the next timers.
Two threads (a worker going idle and the main thread going to sleep)
could each find every context blocked at the same instant; the first
took the flag, the second set it again, took it, and both then waited
for ev.mu. The first moved the clock (1000ms to 1500ms) and woke a
task; the second, on a verdict made before that move, moved the clock
again (1500ms to 2500ms) before the woken task ran. A trace of a
failing run (an in memory log flushed by xo_test_fail; a log written
with a system call per line hid the race, 0 failures in 400 runs)
showed `K from=1000 to=1500` on a worker and `K from=1500 to=2500` on
the main thread with no deadlock check in between.

Fix (2026-10-12): the verdict carries the epoch it was made in. The
check reads tw.vepoch before the counts and stores epoch + 1 in
vpending; tw_kick_once, under ev.mu, moves the clock only when the
verdict's epoch is even and still current, and adds 1 to the epoch
before it fires timers and 1 after (what they woke is queued by then).
A verdict made before or during a move is dropped; the thread that
moved the clock (or the contexts it woke) runs the check again.

Lesson: a flag that records a verdict about shared state ("everything
is idle") is stale as soon as the lock it was made under is released;
whoever acts on it later must check that nothing acted on the same
verdict in between. Trace races into memory and write the trace out
at the failure; tracing with syscalls changes the timing enough to
hide them.
