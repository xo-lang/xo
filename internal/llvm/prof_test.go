package llvm

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestProfCPU: XO_PROF=cpu (decision 0238) on testdata/prof/fib, a
// recursive fib that takes nearly all the CPU, built with each native
// backend: the text summary's top function by flat samples is fib, the
// pprof file is one `go tool pprof` reads, and without XO_PROF the
// program writes no profile. Where the program has DWARF line tables
// (decision 0239: Linux, and darwin's arm64 builds, whose dSYM the
// selfhost/macho link writes), the top row names fib's hottest line in
// main.xo and `go tool pprof -top -lines` shows main.xo lines.
func TestProfCPU(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program per native backend")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the CPU profile needs SIGPROF (darwin, linux)")
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
			res, err := xotool.Build([]string{filepath.Join("testdata", "prof", "fib")}, xotool.Options{Backend: be})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res)
			}
			bin := res.Binaries[xotool.Host()]
			dir := t.TempDir()
			out := filepath.Join(dir, "cpu.pprof")
			cmd := exec.Command(bin)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "XO_PROF=cpu", "XO_PROF_HZ=1000", "XO_PROF_OUT="+out)
			got, err := runCombined(cmd, 2*time.Minute)
			if err != nil {
				t.Fatalf("run: %v\n%s", err, got)
			}
			if !strings.Contains(got, "fib 102334155") || !strings.Contains(got, "xo profile: ") {
				t.Fatalf("output: %s", got)
			}
			txt, err := os.ReadFile(filepath.Join(dir, "cpu.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if f := topFlat(string(txt)); f != "fib" {
				t.Fatalf("top function by flat %q, want fib:\n%s", f, txt)
			}
			lines := runtime.GOOS == "linux" || be == xotool.BackendArm64
			if lines && !strings.Contains(topFlatRow(string(txt)), "main.xo:") {
				t.Fatalf("the top row names no line of main.xo:\n%s", txt)
			}
			if !strings.Contains(string(txt), "samples") {
				t.Fatalf("summary:\n%s", txt)
			}
			if gobin, err := exec.LookPath("go"); err == nil {
				p, err := exec.Command(gobin, "tool", "pprof", "-top", out).CombinedOutput()
				if err != nil || !strings.Contains(string(p), "fib") {
					t.Fatalf("go tool pprof -top: %v\n%s", err, p)
				}
				if lines {
					p, err := exec.Command(gobin, "tool", "pprof", "-top", "-lines", out).CombinedOutput()
					if err != nil || !strings.Contains(string(p), "main.xo:") {
						t.Fatalf("go tool pprof -top -lines: %v\n%s", err, p)
					}
				}
			}
			// off: no XO_PROF, no profile written, nothing said
			off := t.TempDir()
			cmd = exec.Command(bin)
			cmd.Dir = off
			got, err = runCombined(cmd, 2*time.Minute)
			if err != nil || strings.Contains(got, "xo profile") {
				t.Fatalf("without XO_PROF: %v\n%s", err, got)
			}
			if ents, _ := os.ReadDir(off); len(ents) != 0 {
				t.Fatalf("without XO_PROF the program wrote %v", ents)
			}
		})
	}
}

// topFlat is the function of the first row of the summary's "top 30 by
// flat" table.
// topFlatRow is the first row of the summary's "top 30 by flat".
func topFlatRow(txt string) string {
	lines := strings.Split(txt, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "top 30 by flat") && i+2 < len(lines) {
			return lines[i+2]
		}
	}
	return ""
}

func topFlat(txt string) string {
	lines := strings.Split(txt, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "top 30 by flat") && i+2 < len(lines) {
			f := strings.Fields(lines[i+2])
			if len(f) > 0 {
				return f[len(f)-1]
			}
		}
	}
	return ""
}

// runCombined runs cmd with its output and error together, killed after d.
func runCombined(cmd *exec.Cmd, d time.Duration) (string, error) {
	var b strings.Builder
	cmd.Stdout, cmd.Stderr = &b, &b
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return b.String(), err
	case <-time.After(d):
		cmd.Process.Kill()
		<-done
		return b.String(), os.ErrDeadlineExceeded
	}
}
