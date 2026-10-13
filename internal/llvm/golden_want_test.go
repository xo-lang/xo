package llvm_test

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

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
)

// TestGoldenWant builds every golden program with the LLVM backend alone
// and compares its run with want.txt (no Go backend run), for a host
// where the native runtime is being ported (Windows, decision 0197).
// It runs when XO_GOLDEN_WANT=1 and reports one line per program and a
// count of programs that pass, fail, and are unsupported (the build stops
// with XO1504); unsupported programs do not fail the test, and failures
// fail it only with XO_GOLDEN_WANT_STRICT=1, so one run counts them all.
func TestGoldenWant(t *testing.T) {
	if os.Getenv("XO_GOLDEN_WANT") != "1" {
		t.Skip("set XO_GOLDEN_WANT=1 to compare LLVM builds with want.txt alone")
	}
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	strict := os.Getenv("XO_GOLDEN_WANT_STRICT") == "1"
	// XO_GOLDEN_WANT_TARGET=os/arch: build for that target instead
	// (a cross build, build.LLVMCrossTriple) and stop there; "built"
	// counts the programs that compiled and linked
	target := build.Host()
	if v := os.Getenv("XO_GOLDEN_WANT_TARGET"); v != "" {
		goos, goarch, ok := strings.Cut(v, "/")
		if !ok {
			t.Fatalf("XO_GOLDEN_WANT_TARGET=%q: want os/arch", v)
		}
		target = build.Target{OS: goos, Arch: goarch}
	}
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
		if target != build.Host() {
			t.Logf("golden summary (%s/%s built on %s/%s, not run): %d built, %d fail, %d unsupported, of %d",
				target.OS, target.Arch, build.Host().OS, build.Host().Arch, count["built"], count["fail"], count["unsupported"], len(names))
			return
		}
		t.Logf("golden summary (%s/%s): %d pass, %d fail, %d unsupported, of %d",
			build.Host().OS, build.Host().Arch, count["pass"], count["fail"], count["unsupported"], len(names))
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
			want, err := os.ReadFile(filepath.Join(dir, "want_"+build.Host().OS+".txt"))
			if err != nil {
				want, err = os.ReadFile(filepath.Join(dir, "want.txt"))
			}
			if err != nil {
				set("fail (no want.txt)")
				return
			}
			opts := build.Options{Mode: gen.Build, Backend: build.BackendLLVM}
			if target != build.Host() {
				opts.Targets = []build.Target{target}
			}
			res, err := build.Build([]string{dir}, opts)
			if err != nil {
				set("fail (build: " + firstLine(err.Error()) + ")")
				if strict {
					t.Errorf("build: %v", err)
				}
				return
			}
			if res.Failed {
				for _, d := range res.Diagnostics {
					if d.Code == string(diag.TargetUnsupported) {
						set("unsupported (" + d.Message + ")")
						return
					}
				}
				var b strings.Builder
				for _, d := range res.Diagnostics {
					b.WriteString(diag.Human(d) + "; ")
				}
				set("fail (check: " + b.String() + ")")
				if strict {
					t.Errorf("check failed: %s", b.String())
				}
				return
			}
			if target != build.Host() {
				if _, err := os.Stat(res.Binaries[target]); err != nil {
					set("fail (no binary: " + err.Error() + ")")
					return
				}
				set("built")
				return
			}
			got, err := runBinary(res.Binaries[build.Host()], dir)
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
				lc, err := runBinary(res.Binaries[build.Host()], dir, "XO_LEAKCHECK=1")
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
		})
	}
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
