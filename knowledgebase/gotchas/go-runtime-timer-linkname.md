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
build` passes it with the tag `xo_gotimers`, and plain `go test` of
`std/rt` builds `gotimers_off.go` (timers unknown: never report). Nothing
is reported while a timer is pending. The runtime's own concurrent waits
moved off Go timers (`osSleep`: `select(2)` with a timeout, darwin and
linux), or they would hold every check off forever.

Anticipated: a Go release that renames or removes
`runtime.timeSleepUntil` makes every `xo build` fail to link with
`relocation target runtime.timeSleepUntil not defined`. Then point the
linkname at the new name (search `src/runtime/proc.go` for the function
sysmon calls to find the next timer) or drop the tag in
`internal/build/build.go` goBuild, which turns deadlock reports through
Go calls off.
