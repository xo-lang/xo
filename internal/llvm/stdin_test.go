package llvm

import (
	"bytes"
	"errors"
	"io"
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

// TestStdinNative runs testdata/stdin/reads, built with the Go backend and
// the native ones, on the same standard input and compares what they
// print (decisions 0088, 0102): lines with `\r\n` and a last line without
// a newline, read_all after read_line, a read canceled by a deadline
// whose input arrives later, and a read ended by a stop request
// (SIGTERM).
func TestStdinNative(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("builds a program three times")
	}
	dir := filepath.Join("testdata", "stdin", "reads")
	backends := []string{xotool.BackendGo, xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = append(backends, xotool.BackendArm64)
	}
	bins := map[string]string{}
	for _, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
		if err != nil || res.Failed {
			t.Fatalf("build (%s): %v %v", be, err, res)
		}
		bins[be] = res.Binaries[xotool.Host()]
	}
	type feed func(w io.WriteCloser, p *exec.Cmd, out *syncBuf)
	cases := []struct {
		mode string
		in   feed
	}{
		{"lines", func(w io.WriteCloser, _ *exec.Cmd, _ *syncBuf) {
			io.WriteString(w, "a\nb\r\n\nlast\r")
			w.Close()
		}},
		{"all", func(w io.WriteCloser, _ *exec.Cmd, _ *syncBuf) {
			io.WriteString(w, "first\nx\ny\nz")
			w.Close()
		}},
		{"timeout", func(w io.WriteCloser, _ *exec.Cmd, _ *syncBuf) {
			time.Sleep(300 * time.Millisecond)
			io.WriteString(w, "late\n")
			w.Close()
		}},
		{"stop", func(w io.WriteCloser, p *exec.Cmd, out *syncBuf) {
			io.WriteString(w, "one\ntwo\n")
			for i := 0; i < 100 && !strings.Contains(out.String(), "ready"); i++ {
				time.Sleep(20 * time.Millisecond)
			}
			time.Sleep(300 * time.Millisecond)
			p.Process.Signal(syscall.SIGTERM)
			time.Sleep(300 * time.Millisecond)
			w.Close()
		}},
	}
	for _, c := range cases {
		want := ""
		for _, be := range backends {
			cmd := testtmp.Guard(exec.Command(bins[be], c.mode))
			w, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			out := syncBuf{mu: make(chan struct{}, 1)}
			var stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			c.in(w, cmd, &out)
			err = cmd.Wait()
			code := 0
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			}
			got := out.String() + "exit " + string(rune('0'+code%10)) + "\n" + stableStderr(stderr.String())
			if be == xotool.BackendGo {
				want = got
				t.Logf("%s:\n%s", c.mode, got)
				continue
			}
			if got != want {
				t.Errorf("%s %s differs from the Go backend\n--- go\n%s--- %s\n%s", c.mode, be, want, be, got)
			}
		}
	}
}

// syncBuf is a bytes.Buffer safe for a writer and a concurrent reader.
type syncBuf struct {
	mu  chan struct{}
	buf bytes.Buffer
}

func (b *syncBuf) lock() { b.mu <- struct{}{} }

func (b *syncBuf) Write(p []byte) (int, error) {
	b.lock()
	defer func() { <-b.mu }()
	return b.buf.Write(p)
}

func (b *syncBuf) String() string {
	b.lock()
	defer func() { <-b.mu }()
	return b.buf.String()
}
