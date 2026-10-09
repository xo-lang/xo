//go:build xo_gotimers && (darwin || linux)

package rt

import (
	"math"
	"time"
	_ "unsafe" // go:linkname
)

// Go runtime timers (decision 0073). A Go timer that may release a Go
// call (time.AfterFunc calling WaitGroup.Done, a channel of time.After
// that a parked goroutine receives from) runs no goroutine until it
// fires, so no stack shows it. The runtime knows the earliest pending
// one: runtime.timeSleepUntil, which sysmon uses, returns its time (or
// math.MaxInt64 when there is none) on the runtime.nanotime clock.
// Linking it needs -ldflags=-checklinkname=0 and the build tag
// xo_gotimers, which `xo build` passes (internal/build goBuild); plain
// `go build` and `go test` of this package get gotimers_off.go.

//go:linkname runtimeTimeSleepUntil runtime.timeSleepUntil
func runtimeTimeSleepUntil() int64

//go:linkname runtimeNanotime runtime.nanotime
func runtimeNanotime() int64

// goNextTimerRuntime reports whether a Go timer is pending and how long until
// the earliest fires; known is false where timers cannot be seen.
func goNextTimerRuntime() (d time.Duration, pending, known bool) {
	when := runtimeTimeSleepUntil()
	if when == math.MaxInt64 {
		return 0, false, true
	}
	return time.Duration(when - runtimeNanotime()), true, true
}
