package llvm_test

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// The two phase shutdown (core 7.5) on the native backends (decision
// 0069): testdata/signal is built with each native backend and sent
// SIGTERM (on Windows a Ctrl-Break to its own process group,
// signal_windows_test.go), as internal/gen's TestSignal* do with the Go
// backend. Only this test's own child is ever signaled, by the PID exec
// gave it.

func nativeBackends() []string {
	bs := []string{build.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		bs = append(bs, build.BackendArm64)
	}
	return bs
}

var signalBins sync.Map // backend -> func() (string, error)

func signalBinary(t *testing.T, backend string) string {
	t.Helper()
	f, _ := signalBins.LoadOrStore(backend, sync.OnceValues(func() (string, error) {
		res, err := build.Build([]string{filepath.Join("testdata", "signal")}, build.Options{Mode: gen.Build, Backend: backend})
		if err != nil {
			return "", err
		}
		if res.Failed {
			return "", errors.New("check failed")
		}
		return res.Binaries[build.Host()], nil
	}))
	bin, err := f.(func() (string, error))()
	if err != nil {
		t.Fatalf("%s: build: %v", backend, err)
	}
	return bin
}

type nativeChild struct {
	t     *testing.T
	cmd   *exec.Cmd
	lines chan string
	out   strings.Builder
	done  chan struct{}
}

func startNative(t *testing.T, backend string, env ...string) *nativeChild {
	t.Helper()
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found")
	}
	bin := signalBinary(t, backend)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), env...)
	stopAttr(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &nativeChild{t: t, cmd: cmd, lines: make(chan string, 100), done: make(chan struct{})}
	go func() {
		defer close(c.lines)
		r := bufio.NewReader(stdout)
		for {
			line, err := r.ReadString('\n')
			if line != "" {
				c.lines <- line
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		select {
		case <-c.done:
		default:
			cmd.Process.Kill() // our own child, by its PID
			cmd.Wait()
		}
	})
	return c
}

func (c *nativeChild) expect(want string) {
	c.t.Helper()
	select {
	case line, ok := <-c.lines:
		if !ok {
			c.t.Fatalf("output ended, want %q (so far %q)", want, c.out.String())
		}
		c.out.WriteString(line)
		if line != want+"\n" {
			c.t.Fatalf("line %q, want %q (so far %q)", line, want, c.out.String())
		}
	case <-time.After(20 * time.Second):
		c.t.Fatalf("no line %q within 20s (so far %q)", want, c.out.String())
	}
}

func (c *nativeChild) signal() {
	c.t.Helper()
	if err := sendStop(c.cmd.Process); err != nil {
		c.t.Fatal(err)
	}
}

func (c *nativeChild) wait() (int, string) {
	c.t.Helper()
	var rest strings.Builder
	timeout := time.After(20 * time.Second)
	for {
		select {
		case line, ok := <-c.lines:
			if ok {
				rest.WriteString(line)
				continue
			}
			err := c.cmd.Wait()
			close(c.done)
			code := 0
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			} else if err != nil {
				c.t.Fatal(err)
			}
			return code, rest.String()
		case <-timeout:
			c.t.Fatalf("the program did not exit within 20s (output %q)", c.out.String()+rest.String())
		}
	}
}

// TestNativeSignalTwoPhase: the first signal only requests a stop, the
// second cancels main's root context (deferred code runs, main returns
// normally).
func TestNativeSignalTwoPhase(t *testing.T) {
	for _, bk := range nativeBackends() {
		t.Run(bk, func(t *testing.T) {
			t.Parallel()
			c := startNative(t, bk)
			c.expect("ready")
			c.signal()
			c.expect("stop requested true")
			time.Sleep(300 * time.Millisecond)
			select {
			case line := <-c.lines:
				t.Fatalf("output after the first signal: %q", line)
			default:
			}
			c.signal()
			code, rest := c.wait()
			if code != 0 {
				t.Fatalf("exit %d (output %q)", code, rest)
			}
			if rest != "background stopped\ncleanup ran\n" {
				t.Errorf("output after the second signal %q", rest)
			}
		})
	}
}

// TestNativeSignalDrain: a program that finishes its work after the stop
// request exits 0 without being canceled.
func TestNativeSignalDrain(t *testing.T) {
	for _, bk := range nativeBackends() {
		t.Run(bk, func(t *testing.T) {
			t.Parallel()
			c := startNative(t, bk, "MODE=drain")
			c.expect("ready")
			c.signal()
			c.expect("stop requested true")
			c.expect("drained")
			code, rest := c.wait()
			if code != 0 || rest != "background stopped\ncleanup ran\n" {
				t.Fatalf("exit %d, output %q", code, rest)
			}
		})
	}
}

// TestNativeSignalGrace: `os.proc.set_grace(300ms)` cancels a program
// that ignores the stop 300ms after the first signal.
func TestNativeSignalGrace(t *testing.T) {
	for _, bk := range nativeBackends() {
		t.Run(bk, func(t *testing.T) {
			t.Parallel()
			c := startNative(t, bk, "MODE=grace")
			c.expect("ready")
			start := time.Now()
			c.signal()
			c.expect("stop requested true")
			code, rest := c.wait()
			el := time.Since(start)
			if code != 0 || !strings.Contains(rest, "cleanup ran") {
				t.Fatalf("exit %d, output %q", code, rest)
			}
			if el < 250*time.Millisecond || el > 10*time.Second {
				t.Errorf("canceled after %s, want about 300ms", el)
			}
		})
	}
}

// TestNativeSignalThirdExits: a program stuck in shielded cleanup exits
// with status 130 at the third signal.
func TestNativeSignalThirdExits(t *testing.T) {
	for _, bk := range nativeBackends() {
		t.Run(bk, func(t *testing.T) {
			t.Parallel()
			c := startNative(t, bk, "MODE=hung")
			c.expect("ready")
			c.signal()
			c.expect("stop requested true")
			c.signal()
			time.Sleep(300 * time.Millisecond)
			c.signal()
			code, rest := c.wait()
			if code != 130 {
				t.Fatalf("exit %d, want 130 (output %q)", code, rest)
			}
		})
	}
}
