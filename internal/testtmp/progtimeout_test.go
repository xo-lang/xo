package testtmp

import (
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestRunTimed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	t.Setenv("XO_TEST_PROG_TIMEOUT", "300ms")
	if d := ProgTimeout(); d != 300*time.Millisecond {
		t.Fatalf("ProgTimeout = %v, want 300ms", d)
	}
	start := time.Now()
	timedOut, err := RunTimed(Guard(exec.Command("sleep", "30")))
	if !timedOut || err == nil {
		t.Fatalf("sleep 30: timedOut %v, err %v; want a stopped program", timedOut, err)
	}
	if el := time.Since(start); el > 15*time.Second {
		t.Fatalf("stopping took %v", el)
	}
	timedOut, err = RunTimed(Guard(exec.Command("true")))
	if timedOut || err != nil {
		t.Fatalf("true: timedOut %v, err %v", timedOut, err)
	}
	t.Setenv("XO_TEST_PROG_TIMEOUT", "bogus")
	if d := ProgTimeout(); d != DefaultProgTimeout {
		t.Fatalf("ProgTimeout with a bad value = %v, want %v", d, DefaultProgTimeout)
	}
}
