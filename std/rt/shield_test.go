package rt

import (
	"strings"
	"testing"
)

// TestShieldedDeferLimit: a shielded defer that runs longer than 5
// seconds faults with XO-F005 when it returns (core 3.5); 5 seconds or
// less does not, and neither does a test world, whose clock is virtual.
func TestShieldedDeferLimit(t *testing.T) {
	old := shieldClock
	defer func() { shieldClock = old }()
	var now int64
	shieldClock = func() int64 { return now }
	run := func(tk *Task, took int64) (f *FaultErr) {
		defer func() {
			if r := recover(); r != nil {
				f = r.(*FaultErr)
			}
		}()
		tk.Shielded("main.xo:7", func() {
			if tk.shield != 1 {
				t.Errorf("shield depth %d inside the defer", tk.shield)
			}
			now += took
		})
		return nil
	}
	tk := &Task{}
	if f := run(tk, 5e9); f != nil {
		t.Errorf("5s faulted: %v", f.Msg)
	}
	f := run(tk, 6e9)
	if f == nil || f.Code != "XO-F005" || f.Loc != "main.xo:7" || !strings.Contains(f.Msg, "a shielded defer ran for 6s, longer than 5s") {
		t.Fatalf("6s: %+v", f)
	}
	if tk.shield != 0 {
		t.Errorf("shield depth %d after the fault", tk.shield)
	}
	if f := run(&Task{w: &World{}}, 60e9); f != nil {
		t.Errorf("a test world faulted: %v", f.Msg)
	}
}
