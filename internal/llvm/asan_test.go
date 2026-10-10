package llvm_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/llvm"
)

// TestGoldenASan builds the network goldens (net_echo and the http_*
// servers of decision 0074) with AddressSanitizer and
// UndefinedBehaviorSanitizer and runs them with one worker and with one
// per CPU: the output must be want.txt with no sanitizer report. (The
// same goldens run under ThreadSanitizer in TestGoldenTasks.)
func TestGoldenASan(t *testing.T) {
	if testing.Short() {
		t.Skip("builds golden programs with AddressSanitizer")
	}
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "http_*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no http goldens: %v", err)
	}
	dirs = append(dirs, filepath.Join("testdata", "golden", "net_echo"))
	rt := filepath.Join(rtDir, "xo_rt.c")
	for _, dir := range dirs {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			res, ok, err := build.CheckNative([]string{dir}, false)
			if err != nil || !ok {
				t.Fatalf("check: %v", err)
			}
			abs, _ := filepath.Abs(dir)
			prog, err := llvm.Generate(res, llvm.Options{LineRoot: abs})
			if err != nil {
				t.Fatal(err)
			}
			ll := filepath.Join(t.TempDir(), "prog.ll")
			if err := os.WriteFile(ll, prog.IR, 0o644); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(t.TempDir(), "prog")
			flags := append([]string{"-O1", "-g", "-fsanitize=address,undefined", "-fno-sanitize-recover=undefined", "-Wno-override-module"}, build.IOFlags(prog.IR)...)
			if err := build.CompileC(flags, []string{ll, rt}, bin); err != nil {
				if strings.Contains(err.Error(), "unsupported option") || strings.Contains(err.Error(), "sanitizer") {
					t.Skipf("no AddressSanitizer: %v", err)
				}
				t.Fatal(err)
			}
			for _, threads := range []string{"1", ""} {
				cmd := exec.Command(bin)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "ASAN_OPTIONS=detect_leaks=0:halt_on_error=1", "XO_THREADS="+threads)
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				err := cmd.Run()
				code := 0
				var ee *exec.ExitError
				if errors.As(err, &ee) {
					code = ee.ExitCode()
				} else if err != nil {
					t.Fatalf("run: %v", err)
				}
				text := stderr.String()
				if strings.Contains(text, "AddressSanitizer") || strings.Contains(text, "runtime error:") {
					if len(text) > 6000 {
						text = text[:6000]
					}
					t.Fatalf("XO_THREADS=%q: %s", threads, text)
				}
				got := runResult{exit: code, stdout: stdout.String(), stderr: stableStderr(text)}
				if got.String() != string(want) {
					t.Errorf("XO_THREADS=%q: output differs from want.txt\n--- want\n%s\n--- got\n%s", threads, want, got)
				}
			}
		})
	}
}
