//go:build windows

package testtmp

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestJobKillsChildren: a helper process (this test binary again) starts a
// long child and exits without waiting for it; the child must end with it
// (the kill-on-close job, job_windows.go), where it outlived the test.
func TestJobKillsChildren(t *testing.T) {
	if os.Getenv("XO_TESTTMP_JOB_HELPER") == "1" {
		c := exec.Command("ping", "-n", "120", "127.0.0.1")
		if err := c.Start(); err != nil {
			os.Stdout.WriteString("ERR " + err.Error() + "\n")
			os.Exit(1)
		}
		os.Stdout.WriteString("PID " + strconv.Itoa(c.Process.Pid) + "\n")
		os.Exit(0)
	}
	h := exec.Command(os.Args[0], "-test.run=^TestJobKillsChildren$")
	h.Env = append(os.Environ(), "XO_TESTTMP_JOB_HELPER=1")
	out, err := h.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	pid := 0
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) == 2 && f[0] == "PID" {
			pid, _ = strconv.Atoi(f[1])
		}
	}
	_ = h.Wait()
	if pid == 0 {
		t.Fatal("the helper printed no child PID")
	}
	for i := 0; i < 50; i++ {
		b, _ := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/NH").Output()
		if !strings.Contains(string(b), strconv.Itoa(pid)) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("child %d still runs 5 s after its parent ended", pid)
}
