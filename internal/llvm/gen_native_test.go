package llvm

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestGenGoldensNative builds the programs of internal/gen's goldens
// listed in testdata/gen_native.txt with the Go backend, the LLVM
// backend, and (on an arm64 host) the arm64 backend, all without a
// garbage collector for the native ones, and requires the same exit
// status, stdout, and fault messages (stableStderr), and no allocation
// live at exit for programs that exit normally. The list holds every Go
// golden the native backends build whose output does not depend on the
// clock, the network, or the machine; a program that stops building
// natively (XO1201) fails the test.
func TestGenGoldensNative(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("builds every listed program three times")
	}
	names := genNativeList(t)
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join("..", "gen", "testdata", "golden", name)
			goRes := buildAndRun(t, dir, xotool.Options{Backend: xotool.BackendGo})
			for _, be := range backends {
				var env []string
				if be == xotool.BackendLLVM {
					env = goldenEnv(dir) // -O1 (o2Goldens names none of these)
				}
				res, b := buildRun(t, dir, xotool.Options{Backend: be, Env: env})
				if goRes.String() != res.String() {
					t.Errorf("%s backend differs from the Go backend\n--- go\n%s\n--- %s\n%s", be, goRes, be, res)
				}
				if res.exit == 0 {
					leakCheck(t, b.Binaries[xotool.Host()], dir)
				}
			}
		})
	}
}

// genNativeList reads testdata/gen_native.txt: one golden name per line,
// `#` comments.
func genNativeList(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "gen_native.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(l, '#'); i >= 0 {
			l = strings.TrimSpace(l[:i])
		}
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// TestErrorTracesNative runs the internal/gen goldens whose main fails
// with error frames on Go, LLVM, and arm64 and requires the same standard
// error, frame lines included (decisions 0092, 0104; the golden tests drop
// indented lines): only the crash bundle line, which native builds do not
// write, is left out.
func TestErrorTracesNative(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("builds every listed program three times")
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, name := range []string{"error_conversion", "error_frames", "error_frames_borrow", "error_frames_capture", "main_custom_err", "scope_failure", "arena_frames", "error_frames_closures", "os_tasks"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join("..", "gen", "testdata", "golden", name)
			want := fullRun(t, dir, xotool.BackendGo)
			for _, be := range backends {
				if got := fullRun(t, dir, be); got != want {
					t.Errorf("%s differs from the Go backend\n--- go\n%s\n--- %s\n%s", be, want, be, got)
				}
			}
		})
	}
}

// fullRun builds and runs dir on backend be: exit status, stdout, and
// stderr without crash bundle lines.
func fullRun(t *testing.T, dir, be string) string {
	t.Helper()
	res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
	if err != nil || res.Failed {
		t.Fatalf("build (%s): %v", be, err)
	}
	cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()]))
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run: %v", err)
		}
		code = ee.ExitCode()
	}
	var keep []string
	for _, l := range strings.Split(stderr.String(), "\n") {
		if !strings.HasPrefix(l, "crash bundle:") {
			keep = append(keep, l)
		}
	}
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", code, stdout.String(), strings.Join(keep, "\n"))
}
