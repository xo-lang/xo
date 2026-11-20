# xw events arrive late in script tests

Status: Observed (2026-11-19, task #442)

**Symptom.** `stdproven_offload_test.sh` failed now and then on a busy
host: "a shard heavyd moves: the run's own end is not: got [2], want [1]".
It passed when run again.

**Cause.** xwhook.sh's `xw_event` sends each event in a background
subshell, so the caller costs one fork and never waits. The test cleared
its events file with `rm -f`, ran the next case and then slept 1 s. On a
loaded host, the previous run's `stdproven.end rc=0` landed after the
`rm`, so it was counted as this case's. With a stub that writes 2 s late
(`XWSTUB_DELAY=2`), seven checks fail every time.

**Fix.** Give each case its own events file (the stub writes to
`$XW_TEST_EVENTS`), so a late event lands in its own case's file. Then
wait for the expected lines (`evwait N PATTERN`, up to 30 s) rather than
sleeping a fixed time. Any test that reads xw events from a stub should
do the same.
