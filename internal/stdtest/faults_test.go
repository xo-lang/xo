package stdtest

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// faultUpdate rewrites want.txt of the fault cases from the Go backend,
// whose messages and positions are the corpus's oracle (spec/core.md 3.6
// names the slicing rule; std/rt/str.go's StrSlice and BytesSlice name the
// text).
var faultUpdate = flag.Bool("fault-update", false, "rewrite want.txt of the fault cases from the Go backend")

// TestSliceFaults runs the slice fault programs of testdata/faults/slice on
// every corpus backend and compares each run's exit status, stdout and
// fault line with its want.txt. The exit status is the fault's (70), and
// the fault line is the only stderr line compared: the Go backend adds its
// stack frames and a crash bundle path, the native backends a traceback.
//
// The Go backend records want.txt: its messages are the oracle where the
// spec names none, and the native backends must say the same thing.
func TestSliceFaults(t *testing.T) {
	if testing.Short() {
		t.Skip("the fault corpus builds a program per case; it runs outside -short")
	}
	cases, err := filepath.Glob(filepath.Join("testdata", "faults", "slice", "*"))
	if err != nil || len(cases) == 0 {
		t.Fatalf("no fault cases: %v", err)
	}
	xotool.EnableTestBinCache()
	sem := make(chan struct{}, jobs())
	for _, be := range corporaBackends() {
		for _, dir := range cases {
			name := filepath.Base(dir)
			t.Run(be+"/"+name, func(t *testing.T) {
				if why := xotool.Arm64Skip(be); why != "" {
					t.Skip(why)
				}
				if *faultUpdate {
					if be != xotool.BackendGo {
						t.Skip("only the Go backend records want.txt")
					}
					t.Parallel()
					sem <- struct{}{}
					defer func() { <-sem }()
					got := runFaultCase(t, dir, be)
					if err := os.WriteFile(filepath.Join(dir, "want.txt"), []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				wantBytes, err := os.ReadFile(filepath.Join(dir, "want.txt"))
				if err != nil {
					t.Fatalf("%v (record it with -fault-update)", err)
				}
				t.Parallel()
				sem <- struct{}{}
				defer func() { <-sem }()
				got := runFaultCase(t, dir, be)
				if got != string(wantBytes) {
					t.Errorf("%s/%s:\n--- got\n%s--- want\n%s", be, name, got, wantBytes)
				}
			})
		}
	}
}

// runFaultCase builds and runs the program in dir on backend be and returns
// its behavior in the golden layout: the exit status, stdout, and the
// "fault: " stderr line.
func runFaultCase(t *testing.T, dir, be string) string {
	t.Helper()
	res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
	if err != nil {
		t.Fatalf("build %s: %v", be, err)
	}
	if res.Failed {
		t.Fatalf("build %s failed:\n%s", be, res.Output)
	}
	bin := res.Binaries[xotool.Host()]
	if bin == "" {
		t.Fatalf("build %s made no %s binary: %v", be, xotool.Host(), res.Binaries)
	}
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XO_CRASH_DIR="+t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	var ee *exec.ExitError
	if err := cmd.Run(); errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %s: %v", be, err)
	}
	var fault []string
	for _, l := range strings.Split(stderr.String(), "\n") {
		if strings.HasPrefix(l, "fault: ") {
			fault = append(fault, l)
		}
	}
	faultText := ""
	if len(fault) > 0 {
		faultText = strings.Join(fault, "\n") + "\n"
	}
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", code, stdout.String(), faultText)
}
