package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

var sepHalfEnvs sync.Map

// sepHalfEnv is the environment of a separately compiled darwin build with
// the program's own sections (decision 0409, XO_STD_HALF=on), against a
// std store made once per out directory (its cache under out).
func sepHalfEnv(t *testing.T, xo, root, out string) []string {
	t.Helper()
	if e, ok := sepHalfEnvs.Load(out); ok {
		return e.([]string)
	}
	cache := filepath.Join(out, "half-cache")
	base := []string{"XO_STD=" + filepath.Join(root, "std"), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR=" + cache}
	sc := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc=1"))
	sc.Env = append(os.Environ(), base...)
	b, err := sc.Output()
	if err != nil {
		t.Fatalf("xo dev stdcache: %v", err)
	}
	paths := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(paths) < 2 {
		t.Fatalf("xo dev stdcache printed %q", b)
	}
	e := append(base, "XO_WHOLE_BUILD=off", "XO_SEPCOMP=1", "XO_SEPCOMP_STORE="+paths[0], "XO_SEPCOMP_FACTS="+paths[1], "XO_STD_HALF=on")
	sepHalfEnvs.Store(out, e)
	return e
}

// TestStdHalfTools: a separately compiled darwin build with the program's
// code in its own section (decision 0409, _ptext) works with every tool
// that reads the binary: its fault traceback is a whole build's,
// codesign verifies it, lldb stops in a program function with the
// backtrace through std to main, dsymutil's dSYM from the debug map has
// the program's unit, and a CPU profile (XO_PROF) names the program's
// function. TestUnwind's sephalf case covers compiler/debug's unwinder
// and xo dev debug on it.
func TestStdHalfTools(t *testing.T) {
	if runtime.GOOS != "darwin" || !native() {
		t.Skip("darwin/arm64 only")
	}
	if testing.Short() {
		t.Skip("builds std")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out, err := testtmp.Dir("xo-stdhalf-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(out)
	half := sepHalfEnv(t, xo, root, out)
	build := func(prog, name string, extra ...string) string {
		src, err := filepath.Abs(filepath.Join("testdata", "stdstore", prog))
		if err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(out, name)
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "-o", bin, src))
		cmd.Dir = src
		cmd.Env = append(append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off", "XO_INLINE_DENY=prelude.list_map*,cb,main.fn*,spin", "PWD="+src), extra...)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo build %s: %v\n%s", prog, err, b)
		}
		return bin
	}
	tw := build("trace", "trace-whole", "XO_SEPCOMP=0")
	th := build("trace", "trace-half", half...)
	if l, _ := testtmp.Guard(exec.Command("otool", "-l", th)).Output(); !strings.Contains(string(l), "sectname _ptext") || !strings.Contains(string(l), "sectname __text") {
		t.Fatalf("want __text and _ptext sections:\n%s", regexp.MustCompile("sectname \\S+").FindAllString(string(l), -1))
	}
	stderr := func(bin string, env ...string) string {
		cmd := testtmp.Guard(exec.Command(bin))
		cmd.Env = append(os.Environ(), env...)
		var se strings.Builder
		cmd.Stderr = &se
		_ = cmd.Run()
		return se.String()
	}
	tb := func(s string) string {
		if i := strings.Index(s, "traceback:"); i >= 0 {
			return s[i:]
		}
		return ""
	}
	if a, b := tb(stderr(tw)), tb(stderr(th)); a == "" || a != b {
		t.Errorf("traceback: whole build\n%s\nwith _ptext\n%s", a, b)
	}
	if b, err := testtmp.Guard(exec.Command("codesign", "--verify", "--verbose", th)).CombinedOutput(); err != nil {
		t.Errorf("codesign --verify: %v\n%s", err, b)
	}
	if _, err := exec.LookPath("lldb"); err == nil {
		b, _ := testtmp.Guard(exec.Command("lldb", "-b", "-o", "breakpoint set -n cb", "-o", "run", "-o", "bt", "-o", "kill", "--", th)).CombinedOutput()
		if !regexp.MustCompile("frame #0: [^`]*`cb[^\\n]* at main.xo:\\d+").Match(b) || !regexp.MustCompile("`prelude.list_map").Match(b) || !regexp.MustCompile("`main[^\\n]* at main.xo:\\d+").Match(b) {
			t.Errorf("lldb in the program's code:\n%s", b)
		}
	}
	if _, err := exec.LookPath("dsymutil"); err == nil {
		ds := th + ".dSYM"
		if b, err := testtmp.Guard(exec.Command("dsymutil", th, "-o", ds)).CombinedOutput(); err != nil {
			t.Errorf("dsymutil: %v\n%s", err, b)
		} else if d, _ := testtmp.Guard(exec.Command("dwarfdump", "--debug-info", ds)).Output(); !strings.Contains(string(d), "main.xo") || !strings.Contains(string(d), "prelude.xo") {
			t.Errorf("dSYM: want the program's and std's units")
		}
	}
	sp := build("spin", "spin-half", half...)
	raw := filepath.Join(out, "spin.xoraw")
	stderr(sp, "XO_PROF=cpu", "XO_PROF_HZ=1000", "XO_PROF_OUT="+raw)
	pc := testtmp.Guard(exec.Command(xo, "prof", raw))
	pc.Dir = out
	if b, err := pc.CombinedOutput(); err != nil {
		t.Fatalf("xo prof: %v\n%s", err, b)
	}
	txt, _ := os.ReadFile(filepath.Join(out, "spin.cpu.txt"))
	if !regexp.MustCompile(`(?m)\bspin\b`).Match(txt) {
		t.Errorf("the profile does not name spin:\n%s", txt)
	}
}

// TestStdHalfCache: a separately compiled darwin build keeps std's half
// in the back-end cache (decision 0409 stage 2): the first build writes
// it (link.half_save), an edit of a function body links from it, and its
// binary is the one a build with no half makes, byte for byte, and runs;
// so is an edit that adds a Float constant, whichever way it links.
func TestStdHalfCache(t *testing.T) {
	if runtime.GOOS != "darwin" || !native() {
		t.Skip("darwin/arm64 only")
	}
	if testing.Short() {
		t.Skip("builds std")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out, err := testtmp.Dir("xo-stdhalfc-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(out)
	half := sepHalfEnv(t, xo, root, out)
	src := filepath.Join(out, "prog")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	prog := func(body string) {
		t.Helper()
		code := "fn step(x: Int) -> Int {\n  " + body + "\n}\n\nfn main(os: Os) {\n  var t = 0\n  for i in 0..10 {\n    t = t + step(i)\n  }\n  os.stdio.println(\"t ${t}\")\n}\n"
		if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(code), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// a build with the shared cache (the half's), or with a fresh one
	// (the full link's bytes); whether it wrote a half
	build := func(name string, fresh bool) (string, bool) {
		t.Helper()
		// one file name (the code signature names it), a directory each
		if err := os.MkdirAll(filepath.Join(out, name), 0o755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(out, name, "prog")
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "--phase-times", "-o", bin, "."))
		cmd.Dir = src
		cmd.Env = append(append(os.Environ(), "XO_BIN_CACHE=off", "PWD="+src), half...)
		if fresh {
			// the store stays; only the half and the prepare cache go
			for _, d := range []string{"stdhalf", "prep"} {
				os.RemoveAll(filepath.Join(out, "half-cache", d))
			}
		}
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo build %s: %v\n%s", name, err, b)
		}
		return bin, strings.Contains(string(b), "phase link.half_save")
	}
	run := func(bin, want string) {
		t.Helper()
		b, err := testtmp.Guard(exec.Command(bin)).CombinedOutput()
		if err != nil || strings.TrimSpace(string(b)) != want {
			t.Errorf("%s: %v %q, want %q", bin, err, b, want)
		}
	}
	same := func(a, b string) {
		t.Helper()
		if x, y := readFile(t, a), readFile(t, b); string(x) != string(y) {
			t.Errorf("%s differs from the full link %s (%d and %d bytes)", a, b, len(x), len(y))
		}
	}
	prog("x * 2")
	b1, saved := build("p1", false)
	if !saved {
		t.Errorf("the first build wrote no half")
	}
	run(b1, "t 90")
	hs, _ := filepath.Glob(filepath.Join(out, "half-cache", "stdhalf", "*"))
	if len(hs) != 1 {
		t.Errorf("stdhalf entries: %v", hs)
	}
	prog("x * 3 + 1")
	b2, saved := build("p2", false)
	if saved {
		t.Errorf("an edit of a function body linked in full")
	}
	run(b2, "t 145")
	// a Float constant: whether the link takes the half or links in
	// full (a new section kind, new std names), the full link's bytes
	prog("let f = 1.5\n  if Float(x) > f { x } else { x + 1 }")
	b3, _ := build("p3", false)
	run(b3, "t 47")
	b3f, _ := build("p3-full", true)
	same(b3, b3f)
	// the edit's binary from a cache with no half: the same bytes
	prog("x * 3 + 1")
	b2f, _ := build("p2-full", true)
	same(b2, b2f)
}

func readFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
