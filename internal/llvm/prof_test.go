package llvm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestProfCPU: XO_PROF=cpu (decision 0238) on testdata/prof/fib, a
// recursive fib that takes nearly all the CPU, built with each native
// backend: the text summary's top function by flat samples is fib, the
// pprof file is one `go tool pprof` reads, and without XO_PROF the
// program writes no profile. Where the program has DWARF line tables
// (decision 0239: Linux; on darwin the dSYM compiler/macho writes, or
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
			raw := filepath.Join(dir, "prof.xoraw")
			cmd := testtmp.Guard(exec.Command(bin))
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "XO_PROF=cpu", "XO_PROF_HZ=1000", "XO_PROF_OUT="+raw)
			got, err := runCombined(cmd, 2*time.Minute)
			if err != nil {
				t.Fatalf("run: %v\n%s", err, got)
			}
			if !strings.Contains(got, "fib 102334155") || !strings.Contains(got, "xo profile: ") {
				t.Fatalf("output: %s", got)
			}
			// the program wrote the raw profile; xo prof the pprof and
			// the summary (decision 0245)
			xoProf(t, dir, raw, "--cpu-out="+out)
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
			cmd = testtmp.Guard(exec.Command(bin))
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

// TestProfHelperNames: the arm64 backend's counting helpers carry a
// readable local symbol (parallel.xo alias: the helper's key),
// so the CPU profile of testdata/prof/helpers, which spends its time
// dup'ing the elements of a copied List[User], names `dupp User`, not
// its hashed global xo_h... (2026-10-21: the 32-core HTTP server's top
// three entries, 65% of its CPU, were hashes the profile could not
// name).
func TestProfHelperNames(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	if runtime.GOARCH != "arm64" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		t.Skip("the arm64 backend's CPU profile")
	}
	res, err := xotool.Build([]string{filepath.Join("testdata", "prof", "helpers")}, xotool.Options{Backend: xotool.BackendArm64})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res)
	}
	bin := res.Binaries[xotool.Host()]
	dir := t.TempDir()
	raw := filepath.Join(dir, "prof.xoraw")
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XO_PROF=cpu", "XO_PROF_HZ=1000", "XO_PROF_OUT="+raw)
	got, err := runCombined(cmd, 2*time.Minute)
	if err != nil || !strings.Contains(got, "copies 40020000") {
		t.Fatalf("run: %v\n%s", err, got)
	}
	xoProf(t, dir, raw, "--cpu-out="+filepath.Join(dir, "cpu.pprof"))
	txt, err := os.ReadFile(filepath.Join(dir, "cpu.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(txt), "dupp User") {
		t.Fatalf("the profile names no dupp User:\n%s", txt)
	}
	if regexp.MustCompile(`\bxo_h[0-9A-Za-z]{10}\b`).MatchString(string(txt)) {
		t.Fatalf("the profile shows a helper by its hash:\n%s", txt)
	}
}

// TestProfLeafCaller: on the arm64 backend a leaf whose frame record is
// built only in its fault stubs (testdata/prof/leaf's mix, called from
// outer) has the stubs in an unwind region of their own, so the body's
// compact encoding is frameless: every CPU sample in mix has outer as its
// caller (before, darwin's profile had none: the encoding said FRAME and
// the unwinder took outer's frame record for mix's;
// gotchas/arm64-compact-unwind-leaf-fault-frames.md), and a fault in mix
// still unwinds mix, outer, main through the stub's frame.
func TestProfLeafCaller(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	if runtime.GOARCH != "arm64" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		t.Skip("the arm64 backend's unwind tables, with SIGPROF")
	}
	res, err := xotool.Build([]string{filepath.Join("testdata", "prof", "leaf")}, xotool.Options{Backend: xotool.BackendArm64})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res)
	}
	bin := res.Binaries[xotool.Host()]
	dir := t.TempDir()
	raw := filepath.Join(dir, "prof.xoraw")
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XO_PROF=cpu", "XO_PROF_HZ=1000", "XO_PROF_OUT="+raw)
	got, err := runCombined(cmd, 2*time.Minute)
	if err != nil || !strings.Contains(got, "leaf 680656") {
		t.Fatalf("run: %v\n%s", err, got)
	}
	xoProf(t, dir, raw, "--cpu-out="+filepath.Join(dir, "cpu.pprof"))
	txt, err := os.ReadFile(filepath.Join(dir, "cpu.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if f := topFlat(string(txt)); f != "mix" {
		t.Fatalf("top function by flat %q, want mix:\n%s", f, txt)
	}
	if c := cumPercent(string(txt), "outer"); c < 90 {
		t.Fatalf("outer's cumulative share %.1f%%, want at least 90 (every sample in mix has it as the caller):\n%s", c, txt)
	}
	cmd = testtmp.Guard(exec.Command(bin, "fault"))
	cmd.Env = append(os.Environ(), "XO_TRACEBACK=1")
	got, _ = runCombined(cmd, 2*time.Minute)
	i, j, k := strings.Index(got, "  mix ("), strings.Index(got, "  outer ("), strings.Index(got, "  main (")
	if i < 0 || j < i || k < j {
		t.Fatalf("the fault's traceback is not mix, outer, main:\n%s", got)
	}
}

// cumPercent is the cum% of the row of function fn in an `xo prof` text
// summary's "top by cumulative" table (its last column the function), or
// -1.
func cumPercent(txt, fn string) float64 {
	best := -1.0
	for _, l := range strings.Split(txt, "\n") {
		f := strings.Fields(l)
		if len(f) < 7 || f[len(f)-1] != fn {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSuffix(f[4], "%"), 64)
		if err == nil && v > best {
			best = v
		}
	}
	return best
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
				cmd := testtmp.Guard(exec.Command(bin))
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), env...)
				got, err := runCombined(cmd, 2*time.Minute)
				if err != nil || !strings.Contains(got, "held 2000 churned 2000000") {
					t.Fatalf("run %v: %v\n%s", env, err, got)
				}
				return dir, got
			}
			// the raw profile at XO_PROF_OUT's default, as pprof files in dir
			conv := func(dir string, args ...string) {
				xoProf(t, dir, filepath.Join(dir, "xo-prof.xoraw"), args...)
			}
			// rate 1: every allocation
			dir, got := run("XO_PROF=heap", "XO_PROF_HEAP_RATE=1")
			if !strings.Contains(got, "heap samples in ") {
				t.Fatalf("output: %s", got)
			}
			conv(dir)
			txt, err := os.ReadFile(filepath.Join(dir, "xo-prof.heap.txt"))
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
				p, err := exec.Command(gobin, "tool", "pprof", "-top", "-lines", "-sample_index=alloc_space", filepath.Join(dir, "xo-prof.heap.pprof")).CombinedOutput()
				if err != nil || !regexp.MustCompile(`start \S*main\.xo:\d+ \(inline\)`).Match(p) {
					t.Fatalf("go tool pprof -top -lines: no inlined start: %v\n%s", err, p)
				}
			}
			// the default rate (512 KiB): an estimate
			dir, _ = run("XO_PROF=heap")
			conv(dir)
			txt, err = os.ReadFile(filepath.Join(dir, "xo-prof.heap.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if a, _ := heapTotal(string(txt), "allocated: "); a < 20000000 || a > 80000000 {
				t.Fatalf("allocated %d bytes, want about 40 MB:\n%s", a, txt)
			}
			if gobin, err := exec.LookPath("go"); err == nil {
				p, err := exec.Command(gobin, "tool", "pprof", "-top", "-sample_index=alloc_space", filepath.Join(dir, "xo-prof.heap.pprof")).CombinedOutput()
				if err != nil || !strings.Contains(string(p), "churn") {
					t.Fatalf("go tool pprof -top: %v\n%s", err, p)
				}
			}
			// cpu,heap: both profiles, each at its own path
			dir, _ = run("XO_PROF=cpu,heap")
			conv(dir, "--cpu-out=c.pprof", "--heap-out=h.pprof")
			for _, f := range []string{"c.pprof", "c.txt", "h.pprof", "h.txt"} {
				if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
					t.Fatalf("cpu,heap: %v", err)
				}
			}
		})
	}
}

// TestProfGating: decision 0238's gating. A --release build leaves the
// profiler out (prof.start's symbol is not in the binary) and says so
// when XO_PROF asks; --profile-support keeps it; a profile is written
// only to a new file (O_EXCL, no symbolic link followed, mode 0600), so a
// second run with the same output keeps the first file and says so; the
// same holds for xo prof's pprof files (decision 0245).
func TestProfGating(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program three ways")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the profiler runs on darwin and linux")
	}
	be := xotool.BackendLLVM
	if runtime.GOARCH == "arm64" {
		be = xotool.BackendArm64
	} else if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := filepath.Join("testdata", "prof", "fib")
	build := func(release bool, flags ...string) string {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be, Release: release, Flags: flags, Out: filepath.Join(t.TempDir(), "fib")})
		if err != nil || res.Failed {
			t.Fatalf("build: %v %v", err, res)
		}
		return res.Binaries[xotool.Host()]
	}
	has := func(bin string) bool {
		out, err := exec.Command("nm", bin).CombinedOutput()
		if err != nil {
			t.Fatalf("nm: %v\n%s", err, out)
		}
		return strings.Contains(string(out), "prof_2estart") || strings.Contains(string(out), "prof.start")
	}
	run := func(bin, dir string) string {
		cmd := testtmp.Guard(exec.Command(bin))
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "XO_PROF=cpu", "XO_PROF_HZ=1000")
		got, err := runCombined(cmd, 2*time.Minute)
		if err != nil {
			t.Fatalf("run: %v\n%s", err, got)
		}
		return got
	}
	rel := build(true)
	if has(rel) {
		t.Errorf("a --release build has the profiler's prof.start")
	}
	d := t.TempDir()
	if got := run(rel, d); !strings.Contains(got, "built without the profiler") {
		t.Errorf("a --release build with XO_PROF said: %s", got)
	}
	if ents, _ := os.ReadDir(d); len(ents) != 0 {
		t.Errorf("a --release build wrote %v", ents)
	}
	sup := build(true, "--profile-support")
	if !has(sup) {
		t.Errorf("--release --profile-support has no prof.start")
	}
	d = t.TempDir()
	if got := run(sup, d); !strings.Contains(got, "samples in") {
		t.Errorf("--profile-support: %s", got)
	}
	fi, err := os.Lstat(filepath.Join(d, "xo-prof.xoraw"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("the profile's mode is %v, want 0600", fi.Mode().Perm())
	}
	if got := run(sup, d); !strings.Contains(got, "could not write") {
		t.Errorf("a second run over an existing profile: %s", got)
	}
	// named after the input (xo-prof.xoraw: xo-prof.cpu.pprof), and
	// replaced on a rerun; a path given is never overwritten
	xoProf(t, d, filepath.Join(d, "xo-prof.xoraw"))
	if fi, err := os.Lstat(filepath.Join(d, "xo-prof.cpu.pprof")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("xo prof's profile: %v %v, want mode 0600", err, fi)
	}
	if out, err := xoProfRun(d, filepath.Join(d, "xo-prof.xoraw")); err != nil {
		t.Errorf("xo prof again, its default names replaced: %v %s", err, out)
	}
	if out, err := xoProfRun(d, filepath.Join(d, "xo-prof.xoraw"), "--cpu-out="+filepath.Join(d, "xo-prof.cpu.pprof")); err == nil || !strings.Contains(out, "could not write") {
		t.Errorf("xo prof over a profile named with --cpu-out: %v %s", err, out)
	}
	// a symbolic link where the profile goes is not written through
	d = t.TempDir()
	target := filepath.Join(d, "elsewhere")
	if err := os.Symlink(target, filepath.Join(d, "xo-prof.xoraw")); err != nil {
		t.Fatal(err)
	}
	if got := run(sup, d); !strings.Contains(got, "could not write") {
		t.Errorf("a profile path that is a symbolic link: %s", got)
	}
	if _, err := os.Stat(target); err == nil {
		t.Errorf("the profile was written through a symbolic link")
	}
}

// TestProfDrivers: `xo run --profile` and `xo test --profile` (decision
// 0245) leave the pprof files and summaries the program wrote itself
// before: xo-cpu.pprof and xo-cpu.txt in the working directory for run,
// xo-heap-<pid>.pprof and .txt per test process for test, and no raw
// file behind.
func TestProfDrivers(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the profiler runs on darwin and linux")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	abs := func(p string) string {
		a, err := filepath.Abs(p)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	runIn := func(dir string, args ...string) string {
		cmd := testtmp.Guard(exec.Command(xo, args...))
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "PWD="+dir)
		out, err := runCombined(cmd, 5*time.Minute)
		if err != nil {
			t.Fatalf("xo %v: %v\n%s", args, err, out)
		}
		return out
	}
	noRaw := func(dir string) {
		ents, _ := os.ReadDir(dir)
		for _, e := range ents {
			if strings.HasSuffix(e.Name(), ".xoraw") {
				t.Errorf("a raw profile left behind: %s", e.Name())
			}
		}
	}
	d := t.TempDir()
	out := runIn(d, "run", "--profile=cpu", abs(filepath.Join("testdata", "prof", "fib")))
	if !strings.Contains(out, "samples in xo-cpu.pprof and xo-cpu.txt") {
		t.Errorf("xo run --profile=cpu said: %s", out)
	}
	txt, err := os.ReadFile(filepath.Join(d, "xo-cpu.txt"))
	if err != nil || topFlat(string(txt)) != "fib" {
		t.Errorf("xo run --profile=cpu summary: %v\n%s", err, txt)
	}
	noRaw(d)
	d = t.TempDir()
	runIn(d, "test", "--profile=heap", abs(filepath.Join("testdata", "prof", "tested")))
	ents, _ := os.ReadDir(d)
	var pprofs, txts int
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "xo-heap-") && strings.HasSuffix(e.Name(), ".pprof") {
			pprofs++
		}
		if strings.HasPrefix(e.Name(), "xo-heap-") && strings.HasSuffix(e.Name(), ".txt") {
			txts++
		}
	}
	if pprofs == 0 || pprofs != txts {
		t.Errorf("xo test --profile=heap wrote %v", ents)
	}
	noRaw(d)
}

// xoProf runs `xo prof raw args...` in dir (decision 0245) and fails t when
// it fails.
func xoProf(t *testing.T, dir, raw string, args ...string) {
	t.Helper()
	if out, err := xoProfRun(dir, raw, args...); err != nil {
		t.Fatalf("xo prof %s: %v\n%s", raw, err, out)
	}
}

func xoProfRun(dir, raw string, args ...string) (string, error) {
	xo, err := xotool.Path()
	if err != nil {
		return "", err
	}
	cmd := testtmp.Guard(exec.Command(xo, append([]string{"prof", raw}, args...)...))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PWD="+dir)
	out, err := cmd.CombinedOutput()
	return string(out), err
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

// TestProfProcessOnly: XO_PROF applies to the process that reads it (owner,
// 2026-10-21): a profiled program's children do not see it unless
// XO_PROF_CHILDREN=1; and `xo build --profile=cpu` profiles the compiler
// itself into xo-build.cpu.pprof and its .txt.
func TestProfProcessOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the CPU profile needs SIGPROF (darwin, linux)")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	files := map[string]string{
		"xo.mod":  "module example.com/envchild\n",
		"main.xo": "fn main(os: Os) uses (stdio, proc) {\n  match os.proc.cmd(\"env\", []).output() {\n    Ok(o) => os.stdio.print(o.stdout.to_str() ?? \"\")\n    Err(e) => os.stdio.println(\"error ${e}\")\n  }\n}\n",
	}
	for p, s := range files {
		if err := os.WriteFile(filepath.Join(d, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(d, "envchild")
	build := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "-o", bin, "."))
	build.Dir = d
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("xo build: %v\n%s", err, out)
	}
	run := func(env ...string) string {
		t.Helper()
		cmd := testtmp.Guard(exec.Command(bin))
		cmd.Dir = d
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", env, err, out)
		}
		return string(out)
	}
	if got := run("XO_PROF=cpu", "XO_PROF_QUIET=1", "XO_PROF_OUT="+filepath.Join(d, "a.xoraw")); strings.Contains(got, "XO_PROF=") {
		t.Errorf("the child saw XO_PROF:\n%s", got)
	}
	if got := run("XO_PROF=cpu", "XO_PROF_QUIET=1", "XO_PROF_CHILDREN=1", "XO_PROF_OUT="+filepath.Join(d, "b.xoraw")); !strings.Contains(got, "XO_PROF=cpu") {
		t.Errorf("with XO_PROF_CHILDREN=1 the child did not see XO_PROF:\n%s", got)
	}
	// the compiler's own profile
	self := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "--profile=cpu", "-o", bin, "."))
	self.Dir = d
	if out, err := self.CombinedOutput(); err != nil {
		t.Fatalf("xo build --profile=cpu: %v\n%s", err, out)
	}
	for _, f := range []string{"xo-build.cpu.pprof", "xo-build.cpu.txt", "envchild"} {
		if _, err := os.Stat(filepath.Join(d, f)); err != nil {
			t.Errorf("xo build --profile=cpu left no %s: %v", f, err)
		}
	}
}

// TestProfSyscallCaller: a CPU sample in a C library system call (open
// and read of a small file in a loop, testdata/prof/syscall) keeps the
// Xo function that made the call as its caller: past the leaf stub the
// profile walk follows the libc frames' frame records back into the
// program (before, darwin's profile showed `_open` with no caller,
// 2026-10-23).
func TestProfSyscallCaller(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	if runtime.GOARCH != "arm64" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		t.Skip("the arm64 backend's unwind tables, with SIGPROF")
	}
	res, err := xotool.Build([]string{filepath.Join("testdata", "prof", "syscall")}, xotool.Options{Backend: xotool.BackendArm64})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res)
	}
	bin := res.Binaries[xotool.Host()]
	dir := t.TempDir()
	raw := filepath.Join(dir, "prof.xoraw")
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XO_PROF=cpu", "XO_PROF_HZ=1000", "XO_PROF_OUT="+raw)
	got, err := runCombined(cmd, 2*time.Minute)
	if err != nil || !strings.Contains(got, "read 1000000") {
		t.Fatalf("run: %v\n%s", err, got)
	}
	xoProf(t, dir, raw, "--cpu-out="+filepath.Join(dir, "cpu.pprof"))
	txt, err := os.ReadFile(filepath.Join(dir, "cpu.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if c := cumPercent(string(txt), "reader"); c < 80 {
		t.Fatalf("reader's cumulative share %.1f%%, want at least 80 (its system calls' samples have it as their caller):\n%s", c, txt)
	}
}

// TestProfCountsCPU: a CPU profile's samples add up to the CPU time the
// program used (testdata/prof/spin, four tasks spinning on the workers),
// within 10% at 1,000 Hz. On macOS ITIMER_PROF's SIGPROF arrived for
// 16% to 25% of the ticks (gotchas/xo-prof-macos-undercounts-samples.md),
// so macOS samples from a thread of the profiler's own that counts each
// thread's CPU time (prof.xo mac_sampler).
func TestProfCountsCPU(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	if runtime.GOARCH != "arm64" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		t.Skip("the arm64 backend's CPU profile (darwin, linux)")
	}
	res, err := xotool.Build([]string{filepath.Join("testdata", "prof", "spin")}, xotool.Options{Backend: xotool.BackendArm64})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res)
	}
	bin := res.Binaries[xotool.Host()]
	dir := t.TempDir()
	cmd := testtmp.Guard(exec.Command(bin, "4", "400000000"))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XO_PROF=cpu", "XO_PROF_HZ=1000", "XO_PROF_OUT="+filepath.Join(dir, "prof.xoraw"))
	got, err := runCombined(cmd, 2*time.Minute)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, got)
	}
	m := regexp.MustCompile(`xo profile: (\d+) samples`).FindStringSubmatch(got)
	if m == nil {
		t.Fatalf("no sample count:\n%s", got)
	}
	n, _ := strconv.Atoi(m[1])
	cpu := cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()
	want := cpu.Seconds() * 1000
	if r := float64(n) / want; r < 0.9 || r > 1.1 {
		t.Fatalf("%d samples for %v of CPU at 1,000 Hz (%.0f): ratio %.3f, want 0.9 to 1.1", n, cpu, want, r)
	}
	t.Logf("%d samples for %v of CPU", n, cpu)
}
