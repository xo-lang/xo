package llvm

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestTrace: the execution trace (decision 0240) of a program with a
// channel ping-pong, a mutex, a sleep, a select, and a join, on each
// native backend: XO_TRACE writes the raw file (mode 0600, never over an
// existing file or through a symbolic link), `xo trace` turns it into
// JSON that parses and a summary with every reason, and nothing is
// written without XO_TRACE.
func TestTrace(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program per native backend")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the trace is darwin and linux only")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Skip(err)
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, be := range backends {
		t.Run(be, func(t *testing.T) {
			if be == xotool.BackendLLVM {
				if _, err := xotool.Clang(); err != nil {
					t.Skip("clang not found")
				}
			}
			res, err := xotool.Build([]string{filepath.Join("testdata", "trace", "conc")}, xotool.Options{Backend: be})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res)
			}
			bin := res.Binaries[xotool.Host()]
			dir := t.TempDir()
			raw := filepath.Join(dir, "t.raw")
			run := func(env ...string) string {
				cmd := testtmp.Guard(exec.Command(bin))
				cmd.Dir = dir
				cmd.Env = append(append(os.Environ(), "XO_THREADS=4"), env...)
				got, err := runCombined(cmd, 2*time.Minute)
				if err != nil {
					t.Fatalf("run: %v\n%s", err, got)
				}
				if !strings.Contains(got, "sum 19900") || !strings.Contains(got, "late 1") || !strings.Contains(got, "count 202") {
					t.Fatalf("output: %s", got)
				}
				return got
			}
			run("XO_TRACE=" + raw)
			st, err := os.Stat(raw)
			if err != nil {
				t.Fatal(err)
			}
			if st.Mode().Perm() != 0o600 {
				t.Fatalf("mode %v, want 0600", st.Mode().Perm())
			}
			// an existing file is not overwritten
			if got := run("XO_TRACE=" + raw); !strings.Contains(got, "cannot create") {
				t.Fatalf("an existing file: %s", got)
			}
			// nor a symbolic link followed
			link := filepath.Join(dir, "link.raw")
			target := filepath.Join(dir, "target")
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			if got := run("XO_TRACE=" + link); !strings.Contains(got, "cannot create") {
				t.Fatalf("a symbolic link: %s", got)
			}
			if _, err := os.Stat(target); err == nil {
				t.Fatal("the trace was written through a symbolic link")
			}
			// the JSON and the summary
			out := filepath.Join(dir, "t.json")
			sum, err := testtmp.Guard(exec.Command(xo, "trace", raw, "--json", out)).CombinedOutput()
			if err != nil {
				t.Fatalf("xo trace: %v\n%s", err, sum)
			}
			for _, want := range []string{"5 tasks", "0 lost", "chan send", "chan recv", "select", "mutex", "sleep", "join", "runnable latency", "top wakers", "main.xo:"} {
				if !strings.Contains(string(sum), want) {
					t.Fatalf("summary without %q:\n%s", want, sum)
				}
			}
			b, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			var j struct {
				TraceEvents []struct {
					Name string  `json:"name"`
					Ph   string  `json:"ph"`
					Pid  int     `json:"pid"`
					Tid  int     `json:"tid"`
					Ts   float64 `json:"ts"`
				} `json:"traceEvents"`
			}
			if err := json.Unmarshal(b, &j); err != nil {
				t.Fatalf("the JSON does not parse: %v", err)
			}
			kinds := map[string]int{}
			for _, e := range j.TraceEvents {
				kinds[e.Ph]++
			}
			// slices, wake flows (start and finish), and track names
			for _, ph := range []string{"X", "s", "f", "M"} {
				if kinds[ph] == 0 {
					t.Fatalf("no %q events: %v", ph, kinds)
				}
			}
			// off: nothing written, nothing said
			off := t.TempDir()
			cmd := testtmp.Guard(exec.Command(bin))
			cmd.Dir = off
			got, err := runCombined(cmd, 2*time.Minute)
			if err != nil || strings.Contains(got, "xo trace") {
				t.Fatalf("without XO_TRACE: %v\n%s", err, got)
			}
			if ents, _ := os.ReadDir(off); len(ents) != 0 {
				t.Fatalf("without XO_TRACE the program wrote %v", ents)
			}
		})
	}
}
