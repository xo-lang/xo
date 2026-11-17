//go:build unix

package main

import (
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestRunSignalEndsProgram: a SIGHUP sent to `xo run` alone (a closed
// terminal, a supervisor signalling its pid) reaches the program it
// started, so the program does not outlive it (task #155: a looping
// program ran 13 h at 99% CPU with PPID 1 after its `xo run` was
// signalled; gotchas/alarm-xo-run-orphans-program.md). Before the fix
// SIGHUP ended xo by its default action and left the program running.
func TestRunSignalEndsProgram(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"pid ${os.proc.pid()}\")\n  while true {\n    os.clock.sleep(50ms)\n  }\n}\n",
	})
	// a SIGHUP ignored here (nohup) would stay ignored in the processes
	// this test starts: caught here, it is the default there
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	w := xoCmd(t, "run", filepath.Join(dir, "app"))
	cmd := command(t, w[0], w[1:]...)
	var out lockedBuf
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`pid (\d+)`)
	deadline := time.Now().Add(2 * time.Minute)
	var m []string
	for m == nil && time.Now().Before(deadline) {
		m = re.FindStringSubmatch(out.String())
		if m == nil {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if m == nil {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("the program printed no pid\n%s", out.String())
	}
	pid, _ := strconv.Atoi(m[1])
	// the program this test started (through xo run), stopped here
	// should the check fail
	defer syscall.Kill(pid, syscall.SIGKILL)
	if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatal(err)
	}
	// xo's own exit: cmd.Wait would also wait for the output pipe, which
	// a program left running holds open
	done := make(chan error, 1)
	go func() {
		_, err := cmd.Process.Wait()
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		t.Fatalf("xo run did not end after SIGHUP\n%s", out.String())
	}
	gone := time.Now().Add(10 * time.Second)
	for time.Now().Before(gone) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("the program (pid %d) still runs 10 s after its xo run ended on SIGHUP\n%s", pid, out.String())
}
