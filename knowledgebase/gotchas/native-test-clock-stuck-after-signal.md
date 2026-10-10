# A native test hung after FakeProc.signal_after fired

Status: Observed (2026-10-09; internal/gen's `test_signal_after` and
`test_stdin_stop` under `xo test --no-gc` failed with "the test did not
finish within 1m0s of real time", already at v0.8.0)

Symptom: a native test binary that used `os.proc.signal_after(d)` with a
main that sleeps on the virtual clock never finished; a stack sample
showed main parked in `clock_wait` and no worker running.

Cause: the test world moves its virtual clock only when a deadlock check
finds every context blocked (`tw_kick` in std/llrt/xo_testing.c). The
signal is a callback timer: firing it requests the stop but signals no
park, so no context woke, nobody waited again, and no deadlock check ran
to move the clock to main's sleep deadline.

Fix: `tw_kick` loops: when the timers it fired were callbacks only, it
runs the deadlock check again itself (`dl_check_locked`), which moves the
clock on when every context still waits. A callback that wakes a
context (a park signal lowers `dlk.blocked` at once) or queues work
stops the loop. Both goldens now run in cmd/xo TestNoGCTests.
