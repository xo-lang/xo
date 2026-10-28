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

// TestStdStoreUnwind checks decision 0388's std store builds on
// darwin/arm64 (XO_STD_ROOTS=1: std split into atoms, the link keeping
// what the program reaches, a dead function's DWARF cut): a fault in a
// callback std's List.map calls unwinds through map's frames to
// task.isolate as in a normal build (testdata/stdstore/catch), an
// uncaught one's traceback names the same frames (trace), lldb's
// backtrace from the callback names the same functions, and a link
// through the store (XO_STD_STORE, XO_STD_STORE_CHECK) is the parts'
// link byte for byte. The callbacks and map are kept out of line
// (XO_INLINE_DENY) so the frames exist.
func TestStdStoreUnwind(t *testing.T) {
	if runtime.GOOS != "darwin" || !native() {
		t.Skip("darwin/arm64 only")
	}
	if testing.Short() {
		t.Skip("builds std whole")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out, err := testtmp.Dir("xo-stdstore-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(out)
	env := append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BACKEND_CACHE=off", "XO_BACKEND_CACHE_DIR="+filepath.Join(out, "cache"), "XO_BIN_CACHE=off", "XO_INLINE_DENY=prelude.list_map*,cb,main.fn*")
	store := []string{"XO_STD_ROOTS=1", "XO_DCE=off", "XO_PASSES=-hollow"}
	build := func(prog, name string, extra ...string) string {
		src, err := filepath.Abs(filepath.Join("testdata", "stdstore", prog))
		if err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(out, name)
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "-o", bin, src))
		cmd.Dir = src
		cmd.Env = append(append(env, "PWD="+src, "XO_BUILD_DIR="+filepath.Join(out, "bd-"+name)), extra...)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo build %s (%v): %v\n%s", prog, extra, err, b)
		}
		return bin
	}
	run := func(bin string) (string, string) {
		cmd := testtmp.Guard(exec.Command(bin))
		var so, se strings.Builder
		cmd.Stdout = &so
		cmd.Stderr = &se
		_ = cmd.Run()
		return so.String(), se.String()
	}
	// a caught fault: the same output (each fault turned into a value)
	cn, _ := run(build("catch", "catch-normal"))
	cs, _ := run(build("catch", "catch-store", store...))
	if cn != cs || !strings.Contains(cn, "fault callback fault at 3") {
		t.Errorf("caught fault: normal build printed\n%s\nstore build printed\n%s", cn, cs)
	}
	// an uncaught fault: the traceback's frame names
	frames := func(stderr string) []string {
		var fs []string
		for _, l := range strings.Split(stderr, "\n") {
			if strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "   ") {
				fs = append(fs, strings.Fields(l)[0])
			}
		}
		return fs
	}
	tn := build("trace", "trace-normal")
	ts := build("trace", "trace-store", store...)
	_, en := run(tn)
	_, es := run(ts)
	fn, fs := frames(en), frames(es)
	if len(fn) == 0 || strings.Join(fn, " ") != strings.Join(fs, " ") {
		t.Errorf("traceback frames: normal %v, store %v\nstore stderr:\n%s", fn, fs, es)
	}
	// lldb's backtrace from the callback
	if _, err := exec.LookPath("lldb"); err == nil {
		bt := func(bin string) []string {
			b, _ := testtmp.Guard(exec.Command("lldb", "-b", "-o", "breakpoint set -r xo_f_cb", "-o", "run", "-o", "bt", "-o", "kill", "--", bin)).CombinedOutput()
			re := regexp.MustCompile("frame #\\d+: [^`]*`([^ +(]*)")
			var names []string
			for _, m := range re.FindAllStringSubmatch(string(b), -1) {
				names = append(names, m[1])
			}
			return names
		}
		bn, bs := bt(tn), bt(ts)
		if len(bn) == 0 || strings.Join(bn, " ") != strings.Join(bs, " ") {
			t.Errorf("lldb bt: normal %v, store %v", bn, bs)
		}
	}
	// the link through the store: written by the first build, read by
	// the second, which also links the parts and compares
	st := filepath.Join(out, "std.store")
	build("trace", "trace-st1", append(store, "XO_STD_STORE="+st)...)
	if _, err := os.Stat(st); err != nil {
		t.Fatalf("no store written: %v", err)
	}
	build("trace", "trace-st2", append(store, "XO_STD_STORE="+st, "XO_STD_STORE_CHECK=1")...)
}
