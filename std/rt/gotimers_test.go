package rt

import "testing"

// TestGoTimersUnknown: a build that cannot see Go timers (plain go test,
// or a target without gotimers_on.go) never proves a deadlock through a
// Go call (decision 0073).
func TestGoTimersUnknown(t *testing.T) {
	if _, _, known := goNextTimerRuntime(); known {
		t.Skip("this build sees Go timers")
	}
	if ok, retry := goStuckProof(); ok || retry <= 0 {
		t.Errorf("goStuckProof = %v, %v; want false and a retry", ok, retry)
	}
}
