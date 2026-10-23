package xotool

import (
	"bytes"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A child that outlives XO_BUILD_LIMIT is killed with its state in the
// output: the report names lsof's view of it (and `xo dump`'s attempt, which
// a plain sleep cannot answer).
func TestRunWatchedKillsAtTheLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no sleep or lsof")
	}
	t.Setenv("XO_BUILD_LIMIT", "300ms")
	var out bytes.Buffer
	cmd := exec.Command("sleep", "60")
	start := time.Now()
	err := RunWatched(cmd, nil, &out)
	if !errors.Is(err, ErrHung) {
		t.Fatalf("err = %v, want ErrHung", err)
	}
	if d := time.Since(start); d > 50*time.Second {
		t.Fatalf("took %v: the child was not killed", d)
	}
	if !strings.Contains(out.String(), "hung; its state before the kill") {
		t.Fatalf("no report in %q", out.String())
	}
}

func TestRunWatchedNoLimit(t *testing.T) {
	t.Setenv("XO_BUILD_LIMIT", "0")
	var out bytes.Buffer
	if err := RunWatched(exec.Command("true"), nil, &out); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XO_BUILD_LIMIT", "10s")
	if err := RunWatched(exec.Command("true"), nil, &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("output %q for a child that finished", out.String())
	}
}
