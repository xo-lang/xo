//go:build !(xo_gotimers && (darwin || linux))

package rt

import "time"

// goNextTimerRuntime cannot see Go timers in this build (see
// gotimers_on.go): deadlocks through Go calls are never reported.
func goNextTimerRuntime() (d time.Duration, pending, known bool) { return 0, false, false }
