//go:build unix

package testtmp

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestLifelineParentDies: the lifeline's write end is held by another
// process standing in for the test process; when it is killed (as a
// timed out `go test` is), the child behind the wrapper must be gone
// within 10 seconds.
func TestLifelineParentDies(t *testing.T) {
	wrapper, err := lifelineBinary()
	if err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	holder := exec.Command("sleep", "600")
	holder.ExtraFiles = []*os.File{w}
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	w.Close()
	child := exec.Command(wrapper, "sleep", "600")
	child.ExtraFiles = []*os.File{r}
	if err := child.Start(); err != nil {
		holder.Process.Kill()
		t.Fatal(err)
	}
	r.Close()
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case err := <-done:
		holder.Process.Kill()
		holder.Wait()
		t.Fatalf("the child ended while its lifeline was held: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	holder.Process.Kill()
	holder.Wait()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		child.Process.Kill()
		t.Fatal("the child outlived its lifeline by 10 s")
	}
}

// TestLifelineCut: Lifeline's cut ends the child; the child's exit
// status and pid are its own (cmd.Process is the program).
func TestLifelineCut(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo $$; exec sleep 600")
	cut := Lifeline(t, cmd, time.Minute)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 32)
	n, _ := out.Read(b)
	if got := string(b[:n]); got != itoa(cmd.Process.Pid)+"\n" {
		t.Fatalf("the program's pid %q, cmd.Process.Pid %d", got, cmd.Process.Pid)
	}
	cut()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("cut did not end the child within 10 s")
	}
}

// TestLifelineExitStatus: a program's own exit status comes through.
func TestLifelineExitStatus(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 7")
	Lifeline(t, cmd, time.Minute)
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 7 {
		t.Fatalf("exit: %v, want status 7", err)
	}
}

func itoa(n int) string {
	b := []byte{}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
