package llvm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
// (decision 0239: Linux; on darwin the dSYM selfhost/macho writes, or
// that dsymutil writes after a cc link such as the LLVM backend's), the
// top row names fib's hottest line in main.xo and `go tool pprof -top
// -lines` shows main.xo lines.
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
			_, dsym := exec.LookPath("dsymutil")
			lines := runtime.GOOS == "linux" || be == xotool.BackendArm64 || dsym == nil
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

// TestProfHeap: XO_PROF=heap (decision 0238 item 4) on each native
// backend. At rate 1 every allocation is a sample: the 2000 strings a task
// holds at the exit are in use (str_repeat's), the lists main churned are
// allocated (churn's), and start's, a callee inlined into churn, are its
// own row and an (inline) line in pprof; at the default rate the estimate of the bytes
// allocated is near the program's (about 40 MB); cpu,heap writes both;
// go tool pprof reads it.
func TestProfHeap(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program per native backend")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the heap profile needs the unwind tables' walk (darwin, linux)")
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
			res, err := xotool.Build([]string{filepath.Join("testdata", "prof", "heap")}, xotool.Options{Backend: be})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res)
			}
			bin := res.Binaries[xotool.Host()]
			run := func(env ...string) (string, string) {
				dir := t.TempDir()
				cmd := exec.Command(bin)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), env...)
				got, err := runCombined(cmd, 2*time.Minute)
				if err != nil || !strings.Contains(got, "held 2000 churned 2000000") {
					t.Fatalf("run %v: %v\n%s", env, err, got)
				}
				return dir, got
			}
			// rate 1: every allocation
			dir, got := run("XO_PROF=heap", "XO_PROF_HEAP_RATE=1")
			if !strings.Contains(got, "heap samples in ") {
				t.Fatalf("output: %s", got)
			}
			txt, err := os.ReadFile(filepath.Join(dir, "xo-heap.txt"))
			if err != nil {
				t.Fatal(err)
			}
			inuse, objs := heapTotal(string(txt), "in use: ")
			if inuse < 1900000 || inuse > 2200000 || objs < 2000 || objs > 2100 {
				t.Fatalf("in use %d bytes in %d objects, want about 2 MB in 2000:\n%s", inuse, objs, txt)
			}
			// flat goes to the innermost frame: LLVM inlines
			// cabi.obj_rs_with_cap into raw.str_repeat
			if f := heapTop(string(txt), "inuse_space"); !strings.Contains(f, "str_repeat") && !strings.Contains(f, "obj_rs_with_cap") {
				t.Fatalf("top function by inuse_space %q, want raw.str_repeat or the cabi.obj_rs_with_cap inlined into it:\n%s", f, txt)
			}
			if !strings.Contains(heapTable(string(txt), "inuse_space"), "raw.str_repeat") {
				t.Fatalf("no raw.str_repeat by inuse_space:\n%s", txt)
			}
			if !strings.Contains(heapTable(string(txt), "alloc_space"), "churn") {
				t.Fatalf("no churn by alloc_space:\n%s", txt)
			}
			// start is inlined into churn on both backends: its
			// allocations are named from .debug_info (decision 0239)
			if !strings.Contains(heapTable(string(txt), "alloc_space"), "  start\n") {
				t.Fatalf("no start (inlined into churn) by alloc_space:\n%s", txt)
			}
			if gobin, err := exec.LookPath("go"); err == nil {
				p, err := exec.Command(gobin, "tool", "pprof", "-top", "-lines", "-sample_index=alloc_space", filepath.Join(dir, "xo-heap.pprof")).CombinedOutput()
				if err != nil || !regexp.MustCompile(`start \S*main\.xo:\d+ \(inline\)`).Match(p) {
					t.Fatalf("go tool pprof -top -lines: no inlined start: %v\n%s", err, p)
				}
			}
			// the default rate (512 KiB): an estimate
			dir, _ = run("XO_PROF=heap")
			txt, err = os.ReadFile(filepath.Join(dir, "xo-heap.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if a, _ := heapTotal(string(txt), "allocated: "); a < 20000000 || a > 80000000 {
				t.Fatalf("allocated %d bytes, want about 40 MB:\n%s", a, txt)
			}
			if gobin, err := exec.LookPath("go"); err == nil {
				p, err := exec.Command(gobin, "tool", "pprof", "-top", "-sample_index=alloc_space", filepath.Join(dir, "xo-heap.pprof")).CombinedOutput()
				if err != nil || !strings.Contains(string(p), "churn") {
					t.Fatalf("go tool pprof -top: %v\n%s", err, p)
				}
			}
			// cpu,heap: both profiles, each at its own path
			dir, _ = run("XO_PROF=cpu,heap", "XO_PROF_OUT=c.pprof", "XO_PROF_HEAP_OUT=h.pprof")
			for _, f := range []string{"c.pprof", "c.txt", "h.pprof", "h.txt"} {
				if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
					t.Fatalf("cpu,heap: %v", err)
				}
			}
		})
	}
}

// heapTotal reads "<label>N.NNMB in K objects" of a heap summary as bytes
// (to the summary's two decimals) and objects.
func heapTotal(txt, label string) (int64, int64) {
	i := strings.Index(txt, label)
	if i < 0 {
		return -1, -1
	}
	var num float64
	var unit string
	var objs int64
	s := txt[i+len(label):]
	j := 0
	for j < len(s) && (s[j] == '.' || (s[j] >= '0' && s[j] <= '9')) {
		j++
	}
	fmt.Sscan(s[:j], &num)
	k := j
	for k < len(s) && s[k] != ' ' {
		k++
	}
	unit = s[j:k]
	fmt.Sscanf(s[k:], " in %d objects", &objs)
	mult := map[string]float64{"B": 1, "kB": 1024, "MB": 1 << 20, "GB": 1 << 30}[unit]
	return int64(num * mult), objs
}

// heapTable is the "top 30 by <kind>" table of a heap summary.
func heapTable(txt, kind string) string {
	i := strings.Index(txt, "top 30 by "+kind+"\n")
	if i < 0 {
		return ""
	}
	s := txt[i:]
	if j := strings.Index(s[1:], "\ntop 30 by "); j >= 0 {
		s = s[:j+1]
	}
	return s
}

// heapTop is the function of the first row of the "top 30 by <kind>"
// table of a heap summary.
func heapTop(txt, kind string) string {
	lines := strings.Split(heapTable(txt, kind), "\n")
	if len(lines) < 3 {
		return ""
	}
	f := strings.Fields(lines[2])
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
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
