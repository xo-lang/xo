package gen_test

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// concurrentGolden are the golden programs that start tasks; they are also
// run with the Go race detector.
var concurrentGolden = []string{"scope_tasks", "scope_failure", "channels", "mutex_atomic", "task_std", "cancellation", "map_value_semantics"}

// TestGoldenRace runs the concurrent golden programs built with -race and
// expects the same output and no data race reports.
func TestGoldenRace(t *testing.T) {
	if testing.Short() {
		t.Skip("race builds are slow")
	}
	if out, err := exec.Command("go", "env", "CGO_ENABLED").Output(); err != nil || strings.TrimSpace(string(out)) != "1" {
		t.Skip("the race detector needs cgo")
	}
	for _, name := range concurrentGolden {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join("testdata", "golden", name)
			res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Race: true})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			cmd := exec.Command(res.Binaries[build.Host()])
			cmd.Dir = dir
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			cmd.Run()
			if strings.Contains(stderr.String(), "DATA RACE") {
				t.Fatalf("data race:\n%s", stderr.String())
			}
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(want), "--- stdout\n"+stdout.String()+"--- stderr") {
				t.Errorf("stdout differs under -race:\n%s\nwant.txt:\n%s", stdout.String(), want)
			}
		})
	}
}

// TestCrossCompile builds a program for linux/amd64, darwin/arm64,
// windows/amd64, and wasip1/wasm (core 12.1).
func TestCrossCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("cross builds are slow")
	}
	dir := filepath.Join("testdata", "golden", "scope_tasks")
	out := t.TempDir()
	targets, err := build.ParseTargets("linux/amd64,darwin/arm64,windows/amd64,wasi/wasm32")
	if err != nil {
		t.Fatal(err)
	}
	res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Targets: targets, Out: out})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, tg := range targets {
		bin := res.Binaries[tg]
		fi, err := os.Stat(bin)
		if err != nil || fi.Size() == 0 {
			t.Errorf("%s: no binary at %s", tg, bin)
		}
	}
	if len(res.Binaries) != 4 {
		t.Errorf("got %d binaries, want 4", len(res.Binaries))
	}
}

// TestFaultStackPointsAtXo checks that a fault's Go stack trace names the
// .xo file and line of every Xo frame (core 11.5, //line directives).
func TestFaultStackPointsAtXo(t *testing.T) {
	dir := filepath.Join("testdata", "golden", "arith_overflow")
	got, _ := buildAndRun(t, dir, gen.Build)
	if got.exit != 70 {
		t.Fatalf("exit %d, want 70", got.exit)
	}
	// the fault happens in add_all (main.xo:4), called from main (main.xo:16)
	for _, want := range []string{`main\.xo:4\b`, `main\.xo:16\b`, `F_add_all`} {
		if !regexp.MustCompile(want).MatchString(got.stderr) {
			t.Errorf("stack trace does not contain %s:\n%s", want, got.stderr)
		}
	}
	if strings.Contains(got.stderr, ".go:") {
		t.Errorf("stack trace mentions generated Go files:\n%s", got.stderr)
	}
}

// TestSignalCancelsMain sends SIGINT to a running program: the root scope
// is canceled, defer blocks run, and main returns normally (core 7.5).
func TestSignalCancelsMain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no SIGINT on windows")
	}
	res, err := build.Build([]string{filepath.Join("testdata", "signal")}, build.Options{Mode: gen.Build})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res)
	}
	cmd := exec.Command(res.Binaries[build.Host()])
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(stdout)
	line, _ := r.ReadString('\n')
	if line != "ready\n" {
		cmd.Process.Kill()
		t.Fatalf("first line %q", line)
	}
	// the PID belongs to the process this test started
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	rest, _ := io.ReadAll(r)
	err = cmd.Wait()
	if err != nil {
		t.Fatalf("exit: %v (output %q)", err, rest)
	}
	got := string(rest)
	for _, want := range []string{"background stopped", "cleanup ran"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "not reached") {
		t.Errorf("main continued after cancellation: %q", got)
	}
}

// TestHoles builds a program with a hole under --holes: it runs until the
// hole is evaluated, which faults (core 3.7). Without --holes the build is
// rejected.
func TestHoles(t *testing.T) {
	dir := filepath.Join("testdata", "holes")
	res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatalf("a program with a hole built without --holes")
	}
	res, err = build.Build([]string{dir}, build.Options{Mode: gen.Build, Holes: true})
	if err != nil || res.Failed {
		t.Fatalf("build --holes: %v %+v", err, res)
	}
	cmd := exec.Command(res.Binaries[build.Host()])
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 70 {
		t.Fatalf("exit %v, want 70", err)
	}
	if stdout.String() != "before hole 3\n" {
		t.Errorf("stdout %q", stdout.String())
	}
	for _, want := range []string{"[main.xo:4:11] 1 + 2 = 3", "evaluated a hole `???` at main.xo:1:26"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr.String())
		}
	}
}
