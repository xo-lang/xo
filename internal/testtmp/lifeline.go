package testtmp

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Lifeline makes cmd, which the caller starts, a child that cannot
// outlive the test: on Unix it runs behind the lifeline wrapper
// (internal/testtmp/lifeline), which kills it when the lifeline, a pipe
// whose write end only this process holds, reaches end of file: when
// the test closes it, or when the test process is killed or times out
// and runs no deferred cleanup (a network server child blocked in accept
// stayed behind with PPID 1 before). cmd.Process is still the program
// itself, so its signals, exit status, and Kill are unchanged. A
// t.Cleanup closes the lifeline and kills a child still running, and
// the child is killed after d in any case. cmd must have no ExtraFiles
// (the lifeline is file descriptor 3). The result cuts the lifeline, as
// the test process's death would.
func Lifeline(t testing.TB, cmd *exec.Cmd, d time.Duration) (cut func()) {
	t.Helper()
	if len(cmd.ExtraFiles) != 0 {
		t.Fatal("testtmp.Lifeline: cmd has ExtraFiles")
	}
	var w *os.File
	if runtime.GOOS != "windows" {
		wrapper, err := lifelineBinary()
		if err != nil {
			t.Fatalf("testtmp.Lifeline: the wrapper: %v", err)
		}
		r, pw, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		w = pw
		cmd.Args = append([]string{wrapper, cmd.Path}, cmd.Args[1:]...)
		cmd.Path = wrapper
		cmd.ExtraFiles = []*os.File{r}
		t.Cleanup(func() { r.Close() })
	}
	var once sync.Once
	cut = func() {
		once.Do(func() {
			if w != nil {
				w.Close()
			}
		})
	}
	timer := time.AfterFunc(d, func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	})
	t.Cleanup(func() {
		timer.Stop()
		cut()
		if cmd.Process != nil && cmd.ProcessState == nil {
			cmd.Process.Kill()
		}
	})
	return cut
}

var lifeline struct {
	once sync.Once
	path string
	err  error
}

// lifelineBinary builds the wrapper once per process, into the user
// cache under the hash of its source (a rebuild only when it changes);
// $XO_LIFELINE names a prebuilt one instead (test binaries run where
// there is no Go toolchain, as a Docker container).
func lifelineBinary() (string, error) {
	lifeline.once.Do(func() {
		if p := os.Getenv("XO_LIFELINE"); p != "" {
			lifeline.path = p
			return
		}
		_, self, _, _ := runtime.Caller(0)
		src := filepath.Join(filepath.Dir(self), "lifeline")
		text, err := os.ReadFile(filepath.Join(src, "main.go"))
		if err != nil {
			lifeline.err = err
			return
		}
		sum := sha256.Sum256(append(text, runtime.Version()...))
		cache, err := os.UserCacheDir()
		if err != nil {
			cache = os.TempDir()
		}
		dir := filepath.Join(cache, "xo", "lifeline", hex.EncodeToString(sum[:8]))
		bin := filepath.Join(dir, "xo-lifeline")
		if _, err := os.Stat(bin); err == nil {
			lifeline.path = bin
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			lifeline.err = err
			return
		}
		tmp, err := os.CreateTemp(dir, "build-")
		if err != nil {
			lifeline.err = err
			return
		}
		tmp.Close()
		build := exec.Command("go", "build", "-o", tmp.Name(), ".")
		build.Dir = src
		if out, err := build.CombinedOutput(); err != nil {
			os.Remove(tmp.Name())
			lifeline.err = &buildError{err, string(out)}
			return
		}
		// another test process may have built it meanwhile: either is the same
		if err := os.Rename(tmp.Name(), bin); err != nil {
			os.Remove(tmp.Name())
		}
		lifeline.path = bin
	})
	return lifeline.path, lifeline.err
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }
