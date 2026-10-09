# A pending Go timer is invisible to goroutine stacks

Status: Observed (2026-10-08, decision 0073); the linkname breakage is
Anticipated.

Symptom: with the first version of the deadlock proof through Go calls,
a correct program was killed with XO-F001: the test holds a Mutex, a
`sync.WaitGroup.Go` callback waits for it, and the test waits in Go for
a WaitGroup that `time.AfterFunc(200ms, release.Done)` releases. Under
`xo test --explore=10` seed 3 reported `test blocked in Go call
sync.(*WaitGroup).Wait ... for callback 2`.

Cause: a Go timer runs no goroutine until it fires. Every goroutine was
parked in a sync wait, so the stack based proof held, but the runtime's
timer heap still held the function that would end the wait. No public
API (runtime/metrics included, Go 1.26) reports pending timers.

Fix: `std/rt/gotimers_on.go` links `runtime.timeSleepUntil` (the earliest
pending timer, as sysmon uses it) with `go:linkname`. The linker refuses
that pull since Go 1.23 (`invalid reference to runtime.timeSleepUntil`)
unless the program is linked with `-ldflags=-checklinkname=0`, so `xo
build` passes it with the tag `xo_gotimers`, only to programs that bind a
Go package with callback adapters (others cannot deadlock through a Go
call waiting for a callback), and plain `go test` of
`std/rt` builds `gotimers_off.go` (timers unknown: never report). Nothing
is reported while a timer is pending. The runtime's own concurrent waits
moved off Go timers (`osSleep`: `select(2)` with a timeout, darwin and
linux), or they would hold every check off forever.

Second symptom (Observed 2026-10-08): under full suite load (`go test
./...`), the same golden test reported XO-F001 at seed 4 although the
timer check was in place; alone it passed.

Cause: the window after the timer fired. Its function, an Xo callback,
arrived as a pending callback call (no thread yet) and its goroutine ran
the scheduler's decision, which unlocks while it waits for Go's
goroutines to settle and, under load, sleeps in `osSleep`. The
confirmation saw no timer pending, that goroutine "sleeping like a
watcher", every other goroutine parked, and only the old threads, so it
reported. At the false report: `pend 1`, `picking true`, no timer.

Fix: `confirmGoLocked` reports only when the scheduler is still before
and after the proof (`stillLocked`: nothing in `pend` or `pendRet`, not
`picking` or `lending`, no turn handed out in between); `goParked` treats
a goroutine with `waitGoSettled` on its stack as moving; `goStuckProof`
yields between its two looks. To reproduce such windows, slow the settle
with `waitGoSettledHook` (a 20 ms `osSleep` made the golden fail 20 of 20
runs before the fix, 0 of 1000 after; `TestSchedGoTimerArrival`). CPU
burners and `GOMAXPROCS` alone did not reproduce it (0 of 110 runs).

Anticipated: a Go release that renames or removes
`runtime.timeSleepUntil` fails that link (`relocation target
runtime.timeSleepUntil not defined`). `goBuildTimers` then builds again
without the tag and prints one line, `xo build: warning: deadlock
detection through Go callbacks is off: ...`; the program works, it just
never reports deadlocks through Go calls (and never falsely). On that
warning, point the linkname at the new name (search `src/runtime/proc.go`
for the function sysmon calls to find the next timer). The fallback is
tested with the tag `xo_gotimers_missing` (`std/rt/gotimers_missing.go`,
`internal/build/gotimers_test.go`); `XO_GOTIMERS=off` forces the tag off.
