package gen_test

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/testtmp"
)

var updateGolden = flag.Bool("golden-update", false, "rewrite want.txt of the golden programs")

func TestMain(m *testing.M) {
	flag.Parse()
	// identical builds reuse one binary, within and across runs
	build.EnableTestBinCache()
	// a -race program sleeps 1 s at exit by default (the race runtime's
	// atexit_sleep_ms) so other threads can report; reports during the
	// run are unaffected, and the replay tests run their -race build 32
	// times (gotchas/race-binaries-sleep-at-exit.md)
	if _, ok := os.LookupEnv("GORACE"); !ok {
		os.Setenv("GORACE", "atexit_sleep_ms=0")
	}
	dir, err := testtmp.Dir("xo-gen-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("XO_BUILD_DIR", filepath.Join(dir, "build"))
	os.Setenv("XO_CRASH_DIR", filepath.Join(dir, "crash"))
	// `use go` bindings (decision 0041) go to a shared temporary cache,
	// not into testdata
	os.Setenv("XO_GOBIND_DIR", filepath.Join(dir, "gobind"))
	os.MkdirAll(filepath.Join(dir, "crash"), 0o755)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// runResult is the observable behavior of a program run.
type runResult struct {
	exit   int
	stdout string
	stderr string
}

func (r runResult) String() string {
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", r.exit, r.stdout, r.stderr)
}

var timeRE = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z`)

// stableStderr keeps the lines of stderr that do not depend on the
// machine: Go stack frames and crash bundle paths are dropped; Xo error
// frames ("  at ...") are kept; wall clock times become <time>.
func stableStderr(s string) string {
	s = timeRE.ReplaceAllString(s, "<time>")
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "  at ") {
			out = append(out, l)
			continue
		}
		if l == "" || strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") || strings.HasPrefix(l, "crash bundle:") {
			continue
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// goFrameRE matches one Go frame of a fault's stack (function name, then
// its .xo position), as printed by the test runner and crash reports.
var goFrameRE = regexp.MustCompile(`(?m)^ +x_[^ \n]+\n +[^ \n]+\.xo:\d+\n`)

// dropGoFrames removes the Go frame lists of fault stacks. With and
// without --no-mono they legitimately differ: specialized copies have their own Go names
// (`F_max__int64`) and inlined closure literals have no frame. Fault
// messages, locations, and Xo error frames ("  at ...") are kept.
func dropGoFrames(s string) string { return goFrameRE.ReplaceAllString(s, "") }

// buildAndRun builds the program in dir and runs it with the arguments in
// args.txt and the input in stdin.txt.
func buildAndRun(t *testing.T, dir string, mode gen.Mode, extra ...string) (runResult, *build.Result) {
	t.Helper()
	return buildAndRunOpts(t, dir, build.Options{Mode: mode}, extra...)
}

// buildAndRunOpts is buildAndRun with full build options.
func buildAndRunOpts(t *testing.T, dir string, opts build.Options, extra ...string) (runResult, *build.Result) {
	t.Helper()
	res, err := build.Build([]string{dir}, opts)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.Failed {
		var b strings.Builder
		for _, d := range res.Diagnostics {
			b.WriteString(diag.Human(d) + "\n")
		}
		t.Fatalf("check failed:\n%s", b.String())
	}
	var args []string
	if data, err := os.ReadFile(filepath.Join(dir, "args.txt")); err == nil {
		args = strings.Fields(string(data))
	}
	args = append(args, extra...)
	cmd := exec.Command(res.Binaries[build.Host()], args...)
	cmd.Dir = dir
	cmd.Env = goldenEnv()
	if data, err := os.ReadFile(filepath.Join(dir, "env.txt")); err == nil {
		// extra variables, NAME=VALUE per line
		cmd.Env = append(cmd.Env, strings.Fields(string(data))...)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "stdin.txt")); err == nil {
		cmd.Stdin = bytes.NewReader(data)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v", err)
	}
	return runResult{exit: code, stdout: stdout.String(), stderr: stderr.String()}, res
}

// TestGolden builds and runs every program under testdata/golden with the
// default lowering (specialized generics, inlined closure literals) and
// compares exit status, stdout, and the stable lines of stderr with
// want.txt, apart from the Go frame lists of fault stacks (dropGoFrames).
// Programs with test blocks are run with `xo test` semantics when the
// directory name starts with "test_".
func TestGolden(t *testing.T) {
	if *updateGolden {
		t.Skip("want.txt is written by TestGoldenNoMono")
	}
	testGolden(t, false, false)
}

// TestGoldenNoMono builds every golden program with `--no-mono` (one Go
// generic function per Xo generic function) and compares with want.txt
// exactly, Go frames included; -golden-update writes want.txt from this
// lowering. The two lowerings must not differ in behavior. Skipped with
// -short unless updating: -short runs each golden program once, with the
// default lowering (TestGolden).
func TestGoldenNoMono(t *testing.T) {
	if testing.Short() && !*updateGolden {
		t.Skip("alternate lowering; run without -short")
	}
	testGolden(t, true, false)
}

// TestGoldenNoArenas builds every golden program with `--no-arenas` and
// compares with want.txt like TestGolden, which builds with request scoped
// arenas (the default since decision 0066, std/rt/arena.go). Together they
// are the arena differential: both lowerings must print exactly want.txt.
// arena_escape and arena_holes check that values escaping a request (a
// Mutex, a channel, the response, a background task, a Once, a Go
// callback, a stream) survive arena reuse.
func TestGoldenNoArenas(t *testing.T) {
	if *updateGolden {
		t.Skip("want.txt is written by TestGoldenNoMono")
	}
	if testing.Short() {
		t.Skip("alternate lowering; run without -short")
	}
	testGolden(t, false, true)
}

func testGolden(t *testing.T, noMono, noArenas bool) {
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		dir := dir
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			skipOffline(t, dir)
			mode := gen.Build
			if strings.HasPrefix(filepath.Base(dir), "test_") {
				mode = gen.Test
			}
			got, _ := buildAndRunOpts(t, dir, build.Options{Mode: mode, NoMono: noMono, NoArenas: noArenas})
			got.stderr = stableStderr(got.stderr)
			wantPath := filepath.Join(dir, "want.txt")
			if *updateGolden && noMono {
				if err := os.WriteFile(wantPath, []byte(got.String()), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("missing want.txt (run with -golden-update): %v", err)
			}
			gs, ws := got.String(), string(want)
			if !noMono {
				gs, ws = dropGoFrames(gs), dropGoFrames(ws)
			}
			if gs != ws {
				t.Errorf("output differs\n--- got\n%s\n--- want\n%s", got.String(), want)
			}
		})
	}
}

var (
	goModOnce sync.Once
	goModErr  error
)

// goldenEnv is the environment of a golden program: the host's without
// the heap goal settings, which gc_config prints (decision 0065).
func goldenEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k != "GOGC" && k != "GOMEMLIMIT" && k != "XO_GC_MIN_HEAP" {
			env = append(env, kv)
		}
	}
	return env
}

// skipOffline skips a golden program that binds a third party Go module
// (an xo.mod with `require go` lines) when the module cannot be downloaded
// (no network and not in the module cache).
func skipOffline(t *testing.T, dir string) {
	data, err := os.ReadFile(filepath.Join(dir, "xo.mod"))
	if err != nil || !strings.Contains(string(data), "require go ") {
		return
	}
	goModOnce.Do(func() {
		cmd := exec.Command("go", "mod", "download", "github.com/google/uuid@v1.6.0")
		cmd.Dir = os.TempDir()
		cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GO111MODULE=on")
		if out, err := cmd.CombinedOutput(); err != nil {
			goModErr = fmt.Errorf("%v: %s", err, out)
		}
	})
	if goModErr != nil {
		t.Skipf("third party Go module unavailable (offline?): %v", goModErr)
	}
}
