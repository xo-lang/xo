package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdDebugMap: a separately compiled darwin build (decision 0401)
// writes its runtime tables as segment __XODW and no __DWARF, its
// traceback names a whole build's frames, its debug map's N_OSO entries
// name objects that exist (std's parts and the program's kept copies),
// lldb stops in a std function by name with its source line and
// variables, and dsymutil's dSYM from it holds std's subprograms.
func TestStdDebugMap(t *testing.T) {
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
	out, err := testtmp.Dir("xo-dbgmap-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(out)
	cache := filepath.Join(out, "cache")
	env := append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+cache, "XO_BIN_CACHE=off", "XO_INLINE_DENY=prelude.list_map*,cb,main.fn*")
	sc := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc=0"))
	sc.Env = env
	b, err := sc.Output()
	if err != nil {
		t.Fatalf("xo dev stdcache: %v", err)
	}
	paths := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(paths) < 2 {
		t.Fatalf("xo dev stdcache printed %q", b)
	}
	sep := []string{"XO_WHOLE_BUILD=off", "XO_SEPCOMP=1", "XO_SEPCOMP_STORE=" + paths[0], "XO_SEPCOMP_FACTS=" + paths[1]}
	build := func(prog, name string, extra ...string) string {
		src, err := filepath.Abs(filepath.Join("testdata", "stdstore", prog))
		if err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(out, name)
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "-o", bin, src))
		cmd.Dir = src
		cmd.Env = append(append(env, "PWD="+src), extra...)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo build %s (%v): %v\n%s", prog, extra, err, b)
		}
		return bin
	}
	tw := build("trace", "trace-whole", "XO_SEPCOMP=0")
	ts := build("trace", "trace-sep", sep...)
	l, _ := testtmp.Guard(exec.Command("otool", "-l", ts)).Output()
	if !strings.Contains(string(l), "segname __XODW") || strings.Contains(string(l), "segname __DWARF") {
		t.Errorf("segments: want __XODW and no __DWARF:\n%s", regexp.MustCompile("segname \\S+").FindAllString(string(l), -1))
	}
	stderr := func(bin string) string {
		cmd := testtmp.Guard(exec.Command(bin))
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
	if a, b := tb(stderr(tw)), tb(stderr(ts)); a == "" || a != b {
		t.Errorf("traceback: whole build\n%s\nseparately compiled\n%s", a, b)
	}
	// the map's objects
	nm, _ := testtmp.Guard(exec.Command("nm", "-ap", ts)).Output()
	osos := regexp.MustCompile(" OSO (\\S+)").FindAllStringSubmatch(string(nm), -1)
	prog := 0
	for _, m := range osos {
		if _, err := os.Stat(m[1]); err != nil {
			t.Errorf("N_OSO %s: %v", m[1], err)
		}
		if strings.Contains(m[1], "dbgobj") {
			prog++
		}
	}
	if len(osos) < 10 || prog == 0 {
		t.Errorf("N_OSO entries: %d, %d of the program's", len(osos), prog)
	}
	if _, err := exec.LookPath("lldb"); err == nil {
		b, _ := testtmp.Guard(exec.Command("lldb", "-b", "-o", "breakpoint set -r ^prelude\\.list_map", "-o", "run", "-o", "bt 1", "-o", "kill", "--", ts)).CombinedOutput()
		if !regexp.MustCompile("frame #0: [^`]*`prelude.list_map[^\\n]* at prelude.xo:\\d+").Match(b) {
			t.Errorf("lldb in std through the map:\n%s", b)
		}
	}
	if _, err := exec.LookPath("dsymutil"); err == nil {
		ds := ts + ".dSYM"
		if b, err := testtmp.Guard(exec.Command("dsymutil", ts, "-o", ds)).CombinedOutput(); err != nil {
			t.Errorf("dsymutil: %v\n%s", err, b)
		} else if d, _ := testtmp.Guard(exec.Command("dwarfdump", "--debug-info", ds)).Output(); !strings.Contains(string(d), "prelude.xo") {
			t.Errorf("dSYM: no std unit")
		}
	}
}
