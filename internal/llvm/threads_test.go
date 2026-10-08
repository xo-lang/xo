package llvm_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/llvm"
)

// Thread safety of the runtime and of shared reference counts (decision
// 0058). No Xo construct starts a thread on the native backends yet, so
// threads come from C: std/llrt/testdata/rc_threads.c hands shared
// values to pthreads, and -DXO_RT_TEST_THREADS runs whole programs on
// several threads at once. Both run under ThreadSanitizer when clang
// supports it on the host.

var rtDir = filepath.Join("..", "..", "std", "llrt")

// tsanBuild compiles srcs with ThreadSanitizer and extra flags, or skips
// the test when the host's clang cannot.
func tsanBuild(t *testing.T, flags, srcs []string) string {
	t.Helper()
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found")
	}
	bin := filepath.Join(t.TempDir(), "prog")
	err := build.CompileC(append([]string{"-O1", "-g", "-fsanitize=thread"}, flags...), srcs, bin)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported option") || strings.Contains(err.Error(), "sanitizer") {
			t.Skipf("no ThreadSanitizer: %v", err)
		}
		t.Fatal(err)
	}
	return bin
}

// runTSan runs bin and fails on a nonzero exit or a sanitizer report.
func runTSan(t *testing.T, bin, dir string, env ...string) string {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "TSAN_OPTIONS=halt_on_error=1"), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := out.String()
	if err != nil || strings.Contains(text, "ThreadSanitizer") {
		if len(text) > 4000 {
			text = text[:4000]
		}
		t.Fatalf("%s: %v\n%s", filepath.Base(bin), err, text)
	}
	return text
}

// TestRuntimeThreads runs the C stress test of shared counts, the claim
// of in place updates, cross thread frees, the share walkers, and
// immortal literals: under ThreadSanitizer, then optimized with more
// iterations.
func TestRuntimeThreads(t *testing.T) {
	src := filepath.Join(rtDir, "testdata", "rc_threads.c")
	bin := tsanBuild(t, nil, []string{src})
	out := runTSan(t, bin, ".")
	for _, name := range []string{"string", "list", "claim", "append", "box", "queue", "literal"} {
		if !strings.Contains(out, name+": ok") {
			t.Errorf("test %s did not pass:\n%s", name, out)
		}
	}
	if testing.Short() {
		return
	}
	opt := filepath.Join(t.TempDir(), "opt")
	if err := build.CompileC([]string{"-O2", "-DITER=20000"}, []string{src}, opt); err != nil {
		t.Fatal(err)
	}
	runTSan(t, opt, ".")
}

// deepRecursion lists golden programs that recurse hundreds of thousands
// of calls deep: ThreadSanitizer crashes on them even on one thread (its
// own limit, checked 2026-10-08), so they run on four threads without it.
var deepRecursion = map[string]bool{"recursion": true, "rc_closures": true}

// TestGoldenThreads builds golden programs with share probes (every heap
// value marked shared) and runs each on four threads at once
// (-DXO_RT_TEST_THREADS=4) under ThreadSanitizer: the per thread heaps,
// the deferred work lists, the output lock, the atomic count paths of
// generated code, and literals shared by every thread (never written)
// must not race, and every allocation must be freed.
func TestGoldenThreads(t *testing.T) {
	if testing.Short() {
		t.Skip("builds golden programs with ThreadSanitizer")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	rt := filepath.Join(rtDir, "xo_rt.c")
	ran := 0
	for _, dir := range dirs {
		want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(want, []byte("exit 0\n")) {
			continue // a fault exits the process from one thread
		}
		dir := dir
		ran++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			res, ok, err := build.Check([]string{dir}, false)
			if err != nil || !ok {
				t.Fatalf("check: %v", err)
			}
			abs, _ := filepath.Abs(dir)
			prog, err := llvm.Generate(res, llvm.Options{LineRoot: abs, ShareProbes: true})
			if err != nil {
				t.Fatal(err)
			}
			ll := filepath.Join(t.TempDir(), "prog.ll")
			if err := os.WriteFile(ll, prog.IR, 0o644); err != nil {
				t.Fatal(err)
			}
			flags := []string{"-DXO_RT_TEST_THREADS=4", "-Wno-override-module"}
			var bin string
			if deepRecursion[filepath.Base(dir)] {
				bin = filepath.Join(t.TempDir(), "prog")
				if err := build.CompileC(append([]string{"-O1"}, flags...), []string{ll, rt}, bin); err != nil {
					t.Fatal(err)
				}
			} else {
				bin = tsanBuild(t, flags, []string{ll, rt})
			}
			out := runTSan(t, bin, dir, "XO_LEAKCHECK=1")
			if m := leakRE.FindStringSubmatch(out); m == nil || m[1] != "0" {
				t.Errorf("leak check: %q", m)
			}
		})
	}
	if ran == 0 {
		t.Fatal("no golden program exits normally")
	}
}
