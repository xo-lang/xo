//go:build unix

package xotool

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A hung child that leads its own process group (as a testtmp.Guard'ed xo
// does) is killed with what it started: the clang an LLVM build waited on
// ran on at full CPU after the watchdog killed xo alone (2026-11-07). The
// report lists the child's children.
func TestRunWatchedKillsTheGroup(t *testing.T) {
	t.Setenv("XO_BUILD_LIMIT", "500ms")
	pidf := filepath.Join(t.TempDir(), "pid")
	cmd := exec.Command("sh", "-c", `sleep 60 & echo $! > "$1"; wait`, "sh", pidf)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var out bytes.Buffer
	if err := RunWatched(cmd, nil, &out); !errors.Is(err, ErrHung) {
		t.Fatalf("err = %v, want ErrHung", err)
	}
	if !strings.Contains(out.String(), "--- children (pid") || !strings.Contains(out.String(), "sleep 60") {
		t.Fatalf("the report lists no sleep child:\n%s", out.String())
	}
	b, err := os.ReadFile(pidf)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(10 * time.Second); ; {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		if time.Now().After(end) {
			// this test's own grandchild, by the pid it wrote
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("the grandchild %d outlived the watchdog's kill", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
