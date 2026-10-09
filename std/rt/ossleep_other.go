//go:build !darwin && !linux

package rt

import "time"

// osSleep is time.Sleep where no timer free sleep is wired up; there
// goNextTimerRuntime reports that it cannot see Go timers, so deadlocks through
// Go calls are never reported (gotimers.go).
func osSleep(d time.Duration) { time.Sleep(d) }
