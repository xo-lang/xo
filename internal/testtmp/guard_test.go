//go:build unix

package testtmp

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestGuardOutlivesNothing: a process that guards a child which ignores
// SIGTERM and starts a grandchild that ignores it too, then exits with no
// cleanup (as a `go test -timeout` panic does), leaves neither behind:
// the lifeline ends, the group gets SIGTERM, then SIGKILL (four golden
// programs that ignored SIGTERM ran 27 hours after their test,
// 2026-10-18).
func TestGuardOutlivesNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the lifeline wrapper and waits out its grace period")
	}
	if os.Getenv("XO_GUARD_PARENT") == "1" {
		guardParent()
		return
	}
	parent := exec.Command(os.Args[0], "-test.run=^TestGuardOutlivesNothing$")
	parent.Env = append(os.Environ(), "XO_GUARD_PARENT=1")
	out, err := parent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	parent.Stderr = os.Stderr
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	var pids []int
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if f, ok := strings.CutPrefix(sc.Text(), "pids "); ok {
			for _, s := range strings.Fields(f) {
				n, _ := strconv.Atoi(s)
				pids = append(pids, n)
			}
		}
	}
	parent.Wait()
	if len(pids) != 2 {
		t.Fatalf("the parent reported %v, want the child's and the grandchild's pids", pids)
	}
	deadline := time.Now().Add(grace + 10*time.Second)
	for time.Now().Before(deadline) {
		if !alive(pids[0]) && !alive(pids[1]) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, p := range pids {
		if alive(p) {
			syscall.Kill(p, syscall.SIGKILL)
		}
	}
	t.Fatalf("pids %v outlived their guarding process", pids)
}

// grace is the wrapper's (lifeline/main.go).
const grace = 3 * time.Second

// guardParent starts a guarded shell that ignores SIGTERM and starts a
// grandchild that does too, reports both pids, and exits at once.
func guardParent() {
	cmd := Guard(exec.Command("sh", "-c", `trap "" TERM; sh -c 'trap "" TERM; while :; do sleep 1; done' & echo $$ $!; while :; do sleep 1; done`))
	out, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("error", err)
		os.Exit(1)
	}
	if err := cmd.Start(); err != nil {
		fmt.Println("error", err)
		os.Exit(1)
	}
	line, _ := bufio.NewReader(out).ReadString('\n')
	fmt.Println("pids " + strings.TrimSpace(line))
	os.Exit(0)
}
