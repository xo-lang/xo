package build

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/gen"
)

// The view of pending Go timers (decision 0073) is linked only into
// programs with Go callback adapters; without it, or when its link fails,
// a deadlock through a Go call is never reported.

var deadlockProg = filepath.Join("..", "gen", "testdata", "golden", "go_callback_deadlock_real")

// runFor runs bin for at most d and returns its output and whether it
// exited (false: still running at d, then killed).
func runFor(t *testing.T, bin string, d time.Duration) (string, bool) {
	t.Helper()
	cmd := exec.Command(bin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	select {
	case <-done:
		return out.String(), true
	case <-time.After(d):
		cmd.Process.Kill()
		<-done
		return out.String(), false
	}
}

func buildDeadlockProg(t *testing.T) *Result {
	t.Helper()
	t.Setenv("XO_GOBIND_DIR", filepath.Join(t.TempDir(), "gobind"))
	res, err := Build([]string{deadlockProg}, Options{Mode: gen.Build, Out: filepath.Join(t.TempDir(), "prog")})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %+v", err, res)
	}
	return res
}

func TestNeedGoTimers(t *testing.T) {
	cb := &gen.Program{Files: map[string][]byte{"rt/xo_gobind_sync.go": []byte("x(NewGoCallback(tk))")}}
	plain := &gen.Program{Files: map[string][]byte{"rt/xo_gobind_strconv.go": []byte("GoEnter(tk)"), "m/main.go": []byte("NewGoCallback(")}}
	if !needGoTimers(cb) || needGoTimers(plain) {
		t.Errorf("needGoTimers: callbacks %v, none %v", needGoTimers(cb), needGoTimers(plain))
	}
	t.Setenv("XO_GOTIMERS", "off")
	if needGoTimers(cb) {
		t.Error("XO_GOTIMERS=off did not turn it off")
	}
	if !linknameFailed("relocation target runtime.timeSleepUntil not defined") || linknameFailed("main.go:3: undefined: x") {
		t.Error("linknameFailed")
	}
}

// TestGoTimersOn: a program with Go callbacks links the timer view and
// reports its deadlock through a Go call.
func TestGoTimersOn(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go")
	}
	res := buildDeadlockProg(t)
	if len(res.Warnings) > 0 {
		t.Fatalf("warnings: %v", res.Warnings)
	}
	out, exited := runFor(t, res.Binaries[Host()], 20*time.Second)
	if !exited || !strings.Contains(out, "timer: 112") || !strings.Contains(out, "XO-F001") {
		t.Fatalf("exited %v:\n%s", exited, out)
	}
}

// TestGoTimersForcedOff: without the timer view (XO_GOTIMERS=off) the
// deadlock is never reported: the program hangs as before decision 0073,
// with no false report.
func TestGoTimersForcedOff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go")
	}
	t.Setenv("XO_GOTIMERS", "off")
	res := buildDeadlockProg(t)
	out, exited := runFor(t, res.Binaries[Host()], 3*time.Second)
	if exited || !strings.Contains(out, "timer: 112") || strings.Contains(out, "XO-F001") {
		t.Fatalf("exited %v:\n%s", exited, out)
	}
}

// TestGoTimersLinkFallback: when the timer view does not link (a runtime
// symbol missing), the build retries without it and warns once.
func TestGoTimersLinkFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go")
	}
	old := goTimersTags
	goTimersTags = "xo_gotimers,xo_gotimers_missing"
	defer func() { goTimersTags = old }()
	res := buildDeadlockProg(t)
	if len(res.Warnings) != 1 || res.Warnings[0] != GoTimersOff {
		t.Fatalf("warnings: %v", res.Warnings)
	}
	out, exited := runFor(t, res.Binaries[Host()], 3*time.Second)
	if exited || !strings.Contains(out, "timer: 112") || strings.Contains(out, "XO-F001") {
		t.Fatalf("exited %v:\n%s", exited, out)
	}
}
