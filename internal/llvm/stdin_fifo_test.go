//go:build unix

package llvm

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdinFIFO runs testdata/stdin/reads (lines) with standard input
// from a named pipe whose one writer writes and closes, and requires the
// same output as from an anonymous pipe within 10 seconds: on macOS
// neither poll(2) nor kqueue reports a FIFO's end of input, so the reader
// waited forever (gotchas/native-stdin-fifo-eof.md).
func TestStdinFIFO(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	dir := filepath.Join("testdata", "stdin", "reads")
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = append(backends, xotool.BackendArm64)
	}
	const input = "a\nb\r\n\nlast\r"
	for _, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
		if err != nil || res.Failed {
			t.Fatalf("build (%s): %v %v", be, err, res)
		}
		bin := res.Binaries[xotool.Host()]
		// the expected output: the same input from an anonymous pipe
		pipe := exec.Command(bin, "lines")
		testtmp.Lifeline(t, pipe, time.Minute)
		pipe.Stdin = strings.NewReader(input)
		want, err := pipe.Output()
		if err != nil {
			t.Fatalf("%s: from a pipe: %v", be, err)
		}
		fifo := filepath.Join(t.TempDir(), "in")
		if err := syscall.Mkfifo(fifo, 0o600); err != nil {
			t.Skipf("mkfifo: %v", err)
		}
		go func() {
			w, err := os.OpenFile(fifo, os.O_WRONLY, 0)
			if err != nil {
				return
			}
			io.WriteString(w, input)
			w.Close()
		}()
		r, err := os.Open(fifo) // waits for the writer
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "lines")
		testtmp.Lifeline(t, cmd, time.Minute)
		cmd.Stdin = r
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		r.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s: from a FIFO: %v", be, err)
			}
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
			t.Fatalf("%s: no end of input from a FIFO after 10 s (read %q)", be, out.String())
		}
		if out.String() != string(want) {
			t.Fatalf("%s: from a FIFO %q, from a pipe %q", be, out.String(), want)
		}
	}
}
