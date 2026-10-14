package llvm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestGoldenWant builds every golden program with the LLVM backend alone
// and compares its run with want.txt (no Go backend run), for a host
// where the native runtime is being ported (Windows, decision 0197).
// It runs when XO_GOLDEN_WANT=1 and reports one line per program and a
// count of programs that pass, fail, and are unsupported (the build stops
// with XO1504); unsupported programs do not fail the test, and failures
// fail it only with XO_GOLDEN_WANT_STRICT=1, so one run counts them all.
// XO_GOLDEN_WANT_XO=path builds with that xo binary (xo1) instead of the
// one the tests run (internal/xotool).
func TestGoldenWant(t *testing.T) {
	if os.Getenv("XO_GOLDEN_WANT") != "1" {
		t.Skip("set XO_GOLDEN_WANT=1 to compare LLVM builds with want.txt alone")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	strict := os.Getenv("XO_GOLDEN_WANT_STRICT") == "1"
	// XO_GOLDEN_WANT_XO=path: build with that xo instead of the one the
	// tests run (internal/xotool)
	xo1 := os.Getenv("XO_GOLDEN_WANT_XO")
	if xo1 == "" {
		p, err := xotool.Path()
		if err != nil {
			t.Fatal(err)
		}
		xo1 = p
	}
	// XO_GOLDEN_WANT_TARGET=os/arch: build for that target instead
	// (a cross build) and stop there; "built" counts the programs that
	// compiled and linked
	target := xotool.Host()
	if v := os.Getenv("XO_GOLDEN_WANT_TARGET"); v != "" {
		if !strings.Contains(v, "/") {
			t.Fatalf("XO_GOLDEN_WANT_TARGET=%q: want os/arch", v)
		}
		target = v
	}
	hostOS, _, _ := strings.Cut(xotool.Host(), "/")
	var mu sync.Mutex
	outcome := map[string]string{}
	t.Cleanup(func() {
		var names []string
		count := map[string]int{}
		for n, o := range outcome {
			names = append(names, n)
			count[strings.Fields(o)[0]]++
		}
		sort.Strings(names)
		for _, n := range names {
			t.Logf("golden %s: %s", n, outcome[n])
		}
		if target != xotool.Host() {
			t.Logf("golden summary (%s built on %s, not run): %d built, %d fail, %d unsupported, of %d",
				target, xotool.Host(), count["built"], count["fail"], count["unsupported"], len(names))
			return
		}
		t.Logf("golden summary (%s): %d pass, %d fail, %d unsupported, of %d",
			xotool.Host(), count["pass"], count["fail"], count["unsupported"], len(names))
	})
	for _, dir := range dirs {
		dir := dir
		name := filepath.Base(dir)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			set := func(o string) {
				mu.Lock()
				outcome[name] = o
				mu.Unlock()
			}
			// a program whose output names the target (`target.os`) has
			// a want_<os>.txt for the other targets
			want, err := os.ReadFile(filepath.Join(dir, "want_"+hostOS+".txt"))
			if err != nil {
				want, err = os.ReadFile(filepath.Join(dir, "want.txt"))
			}
			if err != nil {
				set("fail (no want.txt)")
				return
			}
			// the shipped xo builds it (a target counts as done when xo
			// builds it, decision 0178)
			bin, out, err := xo1Build(xo1, dir, name, target)
			switch {
			case err != nil && strings.Contains(out, "XO1504"):
				set("unsupported (" + firstLine(out) + ")")
				return
			case err != nil:
				set("fail (build: " + firstLine(out) + ")")
				if strict {
					t.Errorf("xo build: %v\n%s", err, out)
				}
				return
			}
			if target != xotool.Host() {
				set("built")
				return
			}
			runAndCompare(t, bin, dir, want, strict, set)
		})
	}
}

// runAndCompare runs a golden program's binary, compares its run with
// want, and runs the leak check when it exits 0, recording the outcome.
func runAndCompare(t *testing.T, bin, dir string, want []byte, strict bool, set func(string)) {
	got, err := runBinary(bin, dir)
	if err != nil {
		set("fail (run: " + err.Error() + ")")
		if strict {
			t.Errorf("run: %v", err)
		}
		return
	}
	if got.String() != string(want) {
		set(fmt.Sprintf("fail (output differs: exit %d)", got.exit))
		if strict {
			t.Errorf("output differs from want.txt\n--- want\n%s\n--- got\n%s", want, got)
		} else {
			t.Logf("output differs from want.txt\n--- want\n%s\n--- got\n%s", want, got)
		}
		return
	}
	if got.exit == 0 {
		// every allocation freed at exit, as TestGolden requires
		lc, err := runBinary(bin, dir, "XO_LEAKCHECK=1")
		m := leakRE.FindStringSubmatch(lc.stderr)
		if err != nil || m == nil || m[1] != "0" {
			set(fmt.Sprintf("fail (leak check: %v %q)", err, m))
			if strict {
				t.Errorf("leak check: %v %q", err, m)
			}
			return
		}
	}
	set("pass")
}

// xo1Build builds the golden program dir with the xo binary xo1
// (XO_GOLDEN_WANT_XO) and the LLVM backend for target: the binary, the
// build's output, and its error. xo1 reads the working directory from
// PWD, with / separators on Windows (std.md 3.1).
func xo1Build(xo1, dir, name, target string) (string, string, error) {
	out, err := xotool.TempDir("xo-golden-want-") // removed with the test binary's root
	if err != nil {
		return "", "", err
	}
	bin := filepath.Join(out, name)
	if strings.HasPrefix(target, "windows/") {
		bin += ".exe"
	}
	args := []string{"build", "--backend=llvm", "-o", filepath.ToSlash(bin)}
	if target != xotool.Host() {
		args = append(args, "--targets="+target)
	}
	cmd := exec.Command(xo1, append(args, filepath.ToSlash(dir))...)
	wd, _ := os.Getwd()
	cmd.Env = append(os.Environ(), "PWD="+filepath.ToSlash(wd))
	b, err := cmd.CombinedOutput()
	return bin, string(b), err
}

// runBinary runs a golden program's binary as buildRun does, killing it
// after a minute.
func runBinary(bin, dir string, env ...string) (runResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, progArgs(dir)...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), progEnv(dir)...), env...)
	cmd.Stdin = progStdin(dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return runResult{}, fmt.Errorf("timed out after a minute")
	}
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		return runResult{}, err
	}
	return runResult{exit: code, stdout: stdout.String(), stderr: stableStderr(stderr.String())}, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
