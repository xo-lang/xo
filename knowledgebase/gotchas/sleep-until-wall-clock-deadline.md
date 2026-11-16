# A runtime sleep_until deadline on the wall clock parked the task for decades

Observed 2026-11-16 (aws-43; task #95).

Symptom: runtime code that waited with `poll.sleep_until(sysio.clock_now() + d)`
never woke: the task stayed parked, with no error.

Cause: the timers run on poll.now, the monotonic clock (sysio.mono_ns:
CLOCK_UPTIME_RAW on macOS, CLOCK_MONOTONIC on Linux) or the test world's
virtual clock. sysio.clock_now and rt.clock_now are the wall clock (Unix
time), about 56 years past the time since boot, so the deadline was decades
away. sleep_until's comment named rt.clock_now's clock, which is how the
mistake was made.

Fix: the comment names poll.now, and sleep_until faults ("deadline ... is
N s past the timers' clock ... a wall clock time? pass sysio.mono_ns() +
d") when a deadline is more than ten years past poll.now
(poll.WRONG_CLOCK_NS). Write deadlines as `sysio.mono_ns() + d` (or
`poll.now() + d`). Test: internal/llvm TestRuntimeSleepClock (std/internal
rt_probe "sleep_mono" and "sleep_wall"), which timed out on both native
backends before the fix.
