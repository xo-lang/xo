package arm64

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestDebugVars checks decision 0252's variables on darwin/arm64:
// testdata/debugvars built by the arm64 backend, linked by compiler/macho
// and by cc (dsymutil), and by the LLVM backend, has a dSYM that verifies
// and describes sum's parameter k and its locals (and, from the arm64
// backend, the struct Pair with its fields); lldb stopped at main.xo:12
// in the loop prints k, total, and d with the values of the first
// iteration.
func TestDebugVars(t *testing.T) {
	if runtime.GOOS != "darwin" || !native() {
		t.Skip("darwin/arm64 only")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	for _, tool := range []string{"dwarfdump", "dsymutil", "lldb"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out, err := testtmp.Dir("xo-debugvars-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(out)
	src, err := filepath.Abs(filepath.Join("testdata", "debugvars"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		args []string
	}{
		{"macho", []string{"--backend=arm64", "--ld=macho"}},
		{"cc", []string{"--backend=arm64", "--ld=cc"}},
		{"llvm", []string{"--backend=llvm"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			bin := filepath.Join(out, c.name)
			args := append(append([]string{"build"}, c.args...), "-o", bin, ".")
			cmd := testtmp.Guard(exec.Command(xo, args...))
			cmd.Dir = src
			cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+src, "XO_BIN_CACHE=off")
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("xo %v: %v\n%s", args, err, b)
			}
			dsym := bin + ".dSYM"
			if v, err := exec.Command("dwarfdump", "--verify", dsym).CombinedOutput(); err != nil || !strings.Contains(string(v), "No errors.") {
				t.Errorf("dwarfdump --verify: %v\n%s", err, v)
			}
			info, err := exec.Command("dwarfdump", "--debug-info", dsym).CombinedOutput()
			if err != nil {
				t.Fatalf("dwarfdump --debug-info: %v", err)
			}
			wants := []string{
				`DW_TAG_formal_parameter`,
				`DW_AT_name	("k")`,
				`DW_AT_name	("total")`,
				`DW_AT_location	(`,
			}
			if c.name != "llvm" {
				// structs: arm64 only (an LLVM aggregate has no per field
				// dbg.value, decision 0252 stage 3)
				wants = append(wants, `DW_AT_name	("Pair")`, `DW_AT_name	("ok")`)
			}
			for _, want := range wants {
				if !strings.Contains(string(info), want) {
					t.Errorf("the dSYM's .debug_info lacks %s", want)
				}
			}
			b, err := lldb(t, true, "-b", "-o", "breakpoint set -f main.xo -l 12", "-o", "run", "-o", "frame variable", bin)
			if err != nil {
				t.Fatalf("lldb: %v\n%s", err, b)
			}
			for _, want := range []string{"k = 10", "total = 0", "d = 12"} {
				if !bytes.Contains(b, []byte(want)) {
					t.Errorf("lldb's frame variable at main.xo:12 lacks %q:\n%s", want, b)
				}
			}
		})
	}
}

// TestDebugInfoSameCode is decision 0252's gate: debug information does
// not change code generation. Every golden program built by the arm64
// backend with debug information (the default) and without it (-s) has
// the same machine code (otool -t).
func TestDebugInfoSameCode(t *testing.T) {
	if runtime.GOOS != "darwin" || !native() {
		t.Skip("darwin/arm64 only")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	if _, err := exec.LookPath("otool"); err != nil {
		t.Skip("no otool")
	}
	text := func(t *testing.T, bin string) []byte {
		t.Helper()
		b, err := exec.Command("otool", "-t", bin).Output()
		if err != nil {
			t.Fatalf("otool -t %s: %v", bin, err)
		}
		// the first line names the file
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
		return b
	}
	for _, dir := range goldenDirs(t) {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			var bins [2][]byte
			for i, flags := range [][]string{nil, {"-s"}} {
				res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendArm64, Flags: flags})
				if err != nil {
					t.Fatalf("build: %v", err)
				}
				if res.Dir != "" {
					defer os.RemoveAll(res.Dir)
				}
				if res.Failed {
					t.Fatalf("build failed:\n%s", res.Output)
				}
				bins[i] = text(t, res.Binaries[xotool.Host()])
			}
			if !bytes.Equal(bins[0], bins[1]) {
				t.Errorf("the code differs with debug information (%d against %d bytes of otool -t)", len(bins[0]), len(bins[1]))
			}
		})
	}
}

// TestDwarfReader checks decision 0253's DWARF reader (compiler/debug,
// through `xo dev dwarf`) on testdata/debugvars: from the arm64
// backend's dSYM (compiler/macho and dsymutil) it reads sum's variables
// with the address ranges dwarfdump prints for each, and from the LLVM
// backend's, sum inlined into main with k the constant 10; from each, a
// breakpoint at main.xo:11 resolves to lldb's address, and the line
// tables give that address's line back.
func TestDwarfReader(t *testing.T) {
	if runtime.GOOS != "darwin" || !native() {
		t.Skip("darwin/arm64 only")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	if _, err := exec.LookPath("dwarfdump"); err != nil {
		t.Skip("no dwarfdump")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out, err := testtmp.Dir("xo-dwarfreader-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(out)
	src, err := filepath.Abs(filepath.Join("testdata", "debugvars"))
	if err != nil {
		t.Fatal(err)
	}
	// a location range as dwarfdump prints it, and as xo dev dwarf does
	ddRange := regexp.MustCompile(`\[0x([0-9a-f]+), 0x([0-9a-f]+)\)`)
	xoRange := regexp.MustCompile(`^\s+0x([0-9a-f]+)\.\.0x([0-9a-f]+) `)
	norm := func(a, b string) string {
		x, _ := strconv.ParseUint(a, 16, 64)
		y, _ := strconv.ParseUint(b, 16, 64)
		return strconv.FormatUint(x, 16) + ".." + strconv.FormatUint(y, 16)
	}
	for _, c := range []struct {
		name string
		args []string
		want []string
	}{
		{"macho", []string{"--backend=arm64", "--ld=macho"}, []string{"param k: Int line 5", "var total: Int line 9", "var p: Point line 10", "var d: Int line 11", "var pr: Pair line 14", "reg11 (8), reg12 (8)"}},
		{"cc", []string{"--backend=arm64", "--ld=cc"}, []string{"param k: Int line 5", "var total: Int line 9", "var p: Point line 10", "var d: Int line 11"}},
		{"llvm", []string{"--backend=llvm"}, []string{"inlined sum call line 24", "param k: Int line 5", "var total: Int line 9", "=10"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			bin := filepath.Join(out, c.name)
			args := append(append([]string{"build"}, c.args...), "-o", bin, ".")
			cmd := testtmp.Guard(exec.Command(xo, args...))
			cmd.Dir = src
			cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+src, "XO_BIN_CACHE=off")
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("xo %v: %v\n%s", args, err, b)
			}
			dw := filepath.Join(bin+".dSYM", "Contents", "Resources", "DWARF", c.name)
			got, err := testtmp.Guard(exec.Command(xo, "dev", "dwarf", "--func=sum", dw)).CombinedOutput()
			if err != nil {
				t.Fatalf("xo dev dwarf: %v\n%s", err, got)
			}
			for _, w := range c.want {
				if !bytes.Contains(got, []byte(w)) {
					t.Errorf("xo dev dwarf --func=sum lacks %q:\n%s", w, got)
				}
			}
			// the runtime's static words, the debugger's way to its records
			all, err := testtmp.Guard(exec.Command(xo, "dev", "dwarf", dw)).CombinedOutput()
			if err != nil {
				t.Fatalf("xo dev dwarf: %v\n%s", err, all)
			}
			if !regexp.MustCompile(`(?m)^global var xo_static: \[256\]Int line 0\n\s+always \[0x[0-9a-f]+\]$`).Match(all) {
				t.Errorf("xo dev dwarf: no xo_static global with an address:\n%s", lastLines(all, 20))
			}
			// a breakpoint at main.xo:11 (in sum's loop; inlined into main
			// by LLVM) at lldb's address, and the line at that address
			brk, err := testtmp.Guard(exec.Command(xo, "dev", "dwarf", "--break=main.xo:11", dw)).CombinedOutput()
			if err != nil {
				t.Fatalf("xo dev dwarf --break: %v\n%s", err, brk)
			}
			lb, err := lldb(t, false, "-b", "-o", "breakpoint set -f main.xo -l 11", bin)
			if err != nil {
				t.Fatalf("lldb: %v\n%s", err, lb)
			}
			m := regexp.MustCompile(`address = 0x0*([0-9a-f]+)`).FindSubmatch(lb)
			if m == nil {
				t.Fatalf("lldb set no breakpoint:\n%s", lb)
			}
			if got := strings.TrimSpace(string(brk)); got != "0x"+string(m[1]) {
				t.Errorf("xo dev dwarf --break=main.xo:11: %s, lldb: 0x%s", got, m[1])
			}
			at, err := testtmp.Guard(exec.Command(xo, "dev", "dwarf", "--pc=0x"+string(m[1]), dw)).CombinedOutput()
			if err != nil || !strings.HasSuffix(strings.TrimSpace(string(at)), "/main.xo:11") {
				t.Errorf("xo dev dwarf --pc=0x%s: %v %s, want .../main.xo:11", m[1], err, at)
			}
			if c.name == "llvm" {
				return
			}
			// a breakpoint on sum: past its prologue, at its first statement
			// (line 6, the prologue_end row), for lldb and xo debug alike
			fb, err := testtmp.Guard(exec.Command(xo, "dev", "dwarf", "--break=sum", dw)).CombinedOutput()
			if err != nil {
				t.Fatalf("xo dev dwarf --break=sum: %v\n%s", err, fb)
			}
			lf, err := lldb(t, false, "-b", "-o", "breakpoint set -n sum", bin)
			if err != nil {
				t.Fatalf("lldb: %v\n%s", err, lf)
			}
			if !bytes.Contains(lf, []byte("at main.xo:6:")) {
				t.Errorf("lldb's breakpoint on sum is not at its first statement, main.xo:6:\n%s", lf)
			}
			if m := regexp.MustCompile(`address = 0x0*([0-9a-f]+)`).FindSubmatch(lf); m == nil || strings.TrimSpace(string(fb)) != "0x"+string(m[1]) {
				t.Errorf("xo dev dwarf --break=sum: %s, lldb:\n%s", fb, lf)
			}
			// each variable's ranges against dwarfdump's
			dd, err := exec.Command("dwarfdump", "--name=sum", "--show-children", bin+".dSYM").CombinedOutput()
			if err != nil {
				t.Fatalf("dwarfdump: %v\n%s", err, dd)
			}
			want := map[string][]string{}
			name := ""
			for _, l := range strings.Split(string(dd), "\n") {
				if i := strings.Index(l, `DW_AT_name	("`); i >= 0 {
					name = strings.TrimSuffix(l[i+len(`DW_AT_name	("`):], `")`)
				}
				if m := ddRange.FindStringSubmatch(l); m != nil && name != "" {
					want[name] = append(want[name], norm(m[1], m[2]))
				}
			}
			have := map[string][]string{}
			name = ""
			for _, l := range strings.Split(string(got), "\n") {
				f := strings.Fields(l)
				if len(f) >= 2 && (f[0] == "param" || f[0] == "var") {
					name = strings.TrimSuffix(f[1], ":")
				}
				if m := xoRange.FindStringSubmatch(l); m != nil && name != "" {
					have[name] = append(have[name], norm(m[1], m[2]))
				}
			}
			for _, v := range []string{"k", "total", "p", "d", "pr"} {
				if strings.Join(have[v], " ") != strings.Join(want[v], " ") {
					t.Errorf("%s's ranges: xo dev dwarf %v, dwarfdump %v", v, have[v], want[v])
				}
			}
		})
	}
}

// lastLines is the last n lines of b.
func lastLines(b []byte, n int) string {
	ls := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return strings.Join(ls, "\n")
}

// TestUnwind checks decision 0253's unwinder (compiler/debug unwind.xo,
// through `xo dev unwind`) against lldb: testdata/debugvars stopped in sum
// and testdata/unwind/fiber stopped in work on a task that parked on a
// channel and resumed on its own fiber stack, each built by the arm64
// backend (compiler/macho and cc) and by LLVM. lldb records the stopped
// thread (testdata/unwind/snap.py: registers and stack), and the frames
// xo dev unwind finds in it (inlined calls as frames, each with its
// function and line, or symbol and offset) are lldb's own, in order.
func TestUnwind(t *testing.T) {
	if runtime.GOOS != "darwin" || !native() {
		t.Skip("darwin/arm64 only")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out, err := testtmp.Dir("xo-unwind-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(out)
	snapPy, err := filepath.Abs(filepath.Join("testdata", "unwind", "snap.py"))
	if err != nil {
		t.Fatal(err)
	}
	// an lldb frame: its function (arguments dropped) at file:line, or its
	// symbol + offset
	lldbAt := regexp.MustCompile(`^(.*?)(\(.*\))? at ([^ :]+):(\d+)`)
	lldbOff := regexp.MustCompile(`^(.*) \+ (\d+)$`)
	norm := func(rest string) string {
		rest = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(rest), " [inlined]"), " [opt]")
		rest = strings.TrimSuffix(rest, " [opt]")
		if m := lldbAt.FindStringSubmatch(rest); m != nil {
			return m[1] + " at " + m[3] + ":" + m[4]
		}
		if m := lldbOff.FindStringSubmatch(rest); m != nil {
			return m[1] + "+" + m[2]
		}
		return rest
	}
	for _, p := range []struct {
		name, dir, stop string
		locals          []string
		// a frame's function and a variable to print in it, and what the
		// session prints
		fn, print, want string
	}{
		{"sum", "debugvars", "main.xo:12", []string{"k = 10", "total = 0", "d = 12"}, "sum", "k", "k = 10"},
		{"fiber", filepath.Join("unwind", "fiber"), "main.xo:4", []string{"units = 4"}, "tasks.run_claimed", "units", "units = 4"},
	} {
		src, err := filepath.Abs(filepath.Join("testdata", p.dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct {
			name string
			args []string
		}{
			{"macho", []string{"--backend=arm64", "--ld=macho"}},
			{"cc", []string{"--backend=arm64", "--ld=cc"}},
			{"llvm", []string{"--backend=llvm"}},
		} {
			t.Run(p.name+"/"+c.name, func(t *testing.T) {
				bin := filepath.Join(out, p.name+"-"+c.name)
				args := append(append([]string{"build"}, c.args...), "-o", bin, ".")
				cmd := testtmp.Guard(exec.Command(xo, args...))
				cmd.Dir = src
				cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+src, "XO_BIN_CACHE=off")
				if b, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("xo %v: %v\n%s", args, err, b)
				}
				snap := bin + ".snap"
				lb, err := lldb(t, true, "-b", "-o", "command script import "+snapPy, "-o", "breakpoint set -f main.xo -l "+strings.TrimPrefix(p.stop, "main.xo:"), "-o", "run", "-o", "snap "+snap, "-o", "bt", bin)
				if err != nil {
					t.Fatalf("lldb: %v\n%s", err, lb)
				}
				img := filepath.Base(bin) + "`"
				var want []string
				seen := map[string]bool{}
				for _, l := range strings.Split(string(lb), "\n") {
					l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "*"))
					if !strings.HasPrefix(l, "frame #") || !strings.Contains(l, img) {
						continue
					}
					// lldb prints the stop's frame twice (the stop, then bt)
					n := l[:strings.Index(l, ":")]
					if seen[n] {
						want = nil
						seen = map[string]bool{}
					}
					seen[n] = true
					want = append(want, norm(l[strings.Index(l, img)+len(img):]))
				}
				if len(want) == 0 {
					t.Fatalf("lldb printed no frames of %s:\n%s", img, lb)
				}
				dw := filepath.Join(bin+".dSYM", "Contents", "Resources", "DWARF", filepath.Base(bin))
				got, err := testtmp.Guard(exec.Command(xo, "dev", "unwind", "--snap="+snap, "--dwarf="+dw, "--locals", bin)).CombinedOutput()
				if err != nil {
					t.Fatalf("xo dev unwind: %v\n%s", err, got)
				}
				var have []string
				for _, l := range strings.Split(string(got), "\n") {
					if !strings.HasPrefix(l, "#") {
						continue
					}
					f := strings.SplitN(l, " ", 3)
					if len(f) < 3 {
						continue
					}
					rest := f[2]
					if i := strings.LastIndex(rest, " ("); i >= 0 {
						rest = rest[:i]
					}
					rest = strings.Replace(rest, " [inlined]", "", 1)
					if rest == "?" {
						continue
					}
					have = append(have, rest)
				}
				if strings.Join(have, "\n") != strings.Join(want, "\n") {
					t.Errorf("xo dev unwind's frames:\n  %s\nlldb's:\n  %s\n(lldb:\n%s)", strings.Join(have, "\n  "), strings.Join(want, "\n  "), lb)
				}
				for _, w := range p.locals {
					if !bytes.Contains(got, []byte("    "+w+"\n")) {
						t.Errorf("xo dev unwind --locals lacks %q:\n%s", w, got)
					}
				}
				// the batch session on the same thread: select p.fn's frame,
				// print p.print there, set a breakpoint, and refuse to run
				frame := -1
				for i, h := range have {
					if frame < 0 && strings.HasPrefix(h, p.fn+" ") {
						frame = i
					}
				}
				if frame < 0 {
					t.Fatalf("no frame of %s in %v", p.fn, have)
				}
				sess, _ := testtmp.Guard(exec.Command(xo, "dev", "debug", "--snap="+snap, "--dwarf="+dw, "-c", fmt.Sprintf("frame %d", frame), "-c", "print "+p.print, "-c", "b "+p.stop, "-c", "continue", bin)).CombinedOutput()
				for _, w := range []string{"(xo) print " + p.print + "\n" + p.want + "\n", "breakpoint 1: 0x", "error: this session is a recorded thread"} {
					if !bytes.Contains(sess, []byte(w)) {
						t.Errorf("xo dev debug lacks %q:\n%s", w, sess)
					}
				}
				// the same with --json (spec 11.5): one object a line, v 1
				js, _ := testtmp.Guard(exec.Command(xo, "dev", "debug", "--json", "--snap="+snap, "--dwarf="+dw, "-c", fmt.Sprintf("frame %d", frame), "-c", "print "+p.print, "-c", "bt", bin)).CombinedOutput()
				var objs []map[string]any
				for _, l := range strings.Split(strings.TrimSpace(string(js)), "\n") {
					var o map[string]any
					if err := json.Unmarshal([]byte(l), &o); err != nil {
						t.Fatalf("xo dev debug --json: a line is not JSON (%v): %s", err, l)
					}
					if o["v"] != 1.0 || o["ok"] != true {
						t.Errorf("xo dev debug --json: want v 1 and ok: %s", l)
					}
					objs = append(objs, o)
				}
				if len(objs) != 3 {
					t.Fatalf("xo dev debug --json: want 3 lines:\n%s", js)
				}
				if got := objs[1]["text"]; got != strings.TrimPrefix(p.want, p.print+" = ") {
					t.Errorf("print %s: text %v, want %s", p.print, got, p.want)
				}
				frames, _ := objs[2]["frames"].([]any)
				if len(frames) <= frame || frames[frame].(map[string]any)["selected"] != true {
					t.Errorf("backtrace: frame %d is not the selected one:\n%s", frame, js)
				}
				// the line protocol on standard input: an empty line repeats
				// the last command; quit ends it
				in := testtmp.Guard(exec.Command(xo, "dev", "debug", "--json", "--snap="+snap, "--dwarf="+dw, bin))
				in.Stdin = strings.NewReader(fmt.Sprintf("frame %d\nprint %s\n\nquit\nbt\n", frame, p.print))
				lp, err := in.CombinedOutput()
				if err != nil {
					t.Fatalf("xo dev debug on standard input: %v\n%s", err, lp)
				}
				ls := strings.Split(strings.TrimSpace(string(lp)), "\n")
				if len(ls) != 4 || !strings.Contains(ls[1], `"cmd":"print"`) || ls[1] != ls[2] || !strings.Contains(ls[3], `"cmd":"quit"`) {
					t.Errorf("xo dev debug on standard input: want frame, print twice, quit (and nothing after quit):\n%s", lp)
				}
			})
		}
	}
}

// TestDebugTypes checks decision 0252 stage 4's types on darwin/arm64:
// testdata/debugtypes built by the arm64 backend (compiler/macho and cc)
// and by LLVM has a dSYM that verifies, and lldb stopped in report (inlined into
// main) reads main's List[Int] through its object (count, length,
// capacity), a Map[Str, Int] as its struct of lists, a Str's bytes
// through its object, and an enum as its tag's name and the union of its
// variants.
func TestDebugTypes(t *testing.T) {
	if runtime.GOOS != "darwin" || !native() {
		t.Skip("darwin/arm64 only")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	for _, tool := range []string{"dwarfdump", "lldb"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out, err := testtmp.Dir("xo-debugtypes-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(out)
	src, err := filepath.Abs(filepath.Join("testdata", "debugtypes"))
	if err != nil {
		t.Fatal(err)
	}
	arm64Cmds := []string{"frame variable *xs", "frame variable m", "frame variable *it.name.obj", "frame variable sh"}
	arm64Wants := []string{
		"(List[Int].obj) *xs = (count = 1, len = 3, cap = 4",
		"(Map[Str, Int]) m = {",
		"keys = 0x",
		// it.name, a literal: no retain keeps its word in a register
		// (an immortal field), so its location is the constant itself,
		// DW_OP_addrx of the static object (dwvars.xo)
		`bytes = "box`,
		"(Shape) sh = {",
		"tag = Rect",
		"Rect = (w = 4, h = 5)",
	}
	for _, c := range []struct {
		name  string
		args  []string
		cmds  []string
		wants []string
		// what lldb must not print: a stale value of a variable
		nots []string
	}{
		{"macho", []string{"--backend=arm64", "--ld=macho"}, arm64Cmds, arm64Wants, nil},
		// not the string's bytes: dsymutil relocates an address in a
		// location list as the function's
		// (gotchas/dsymutil-loclist-address-function-offset.md)
		{"cc", []string{"--backend=arm64", "--ld=cc"}, arm64Cmds, append(append([]string{}, arm64Wants[:3]...), arm64Wants[4:]...), nil},
		// LLVM (compiler/llvm debug.xo): a value the function builds (a
		// struct or enum literal) by its parts, a list through its object,
		// and the parts of a parameter the code extracts (report's name.len,
		// m.n); not the other parts of an aggregate that arrives whole (lldb
		// prints such an empty piece as 0 or NULL, gdb as <optimized out>),
		// not main's m, a call's result it never splits, and not the
		// string's address (gotchas/llvm-aggregate-dbg-value.md). main's m
		// was the zero map of `{}` until a later binding ended that location
		{"llvm", []string{"--backend=llvm"}, []string{"frame variable *xs", "frame variable m", "frame variable it", "frame select 0", "frame variable name m"},
			[]string{"(List[Int].obj) *xs = (count = 1, len = 3, cap = 4", "(Item) it = {", "len = 3", "tags = 0x", "(Str) name = {", "len = 5", "n = 2"},
			[]string{"(Map[Str, Int]) m = {\n  keys = NULL\n  vals = NULL\n  live = NULL\n  slots = NULL\n  n = 0"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			bin := filepath.Join(out, c.name)
			args := append(append([]string{"build"}, c.args...), "-o", bin, ".")
			cmd := testtmp.Guard(exec.Command(xo, args...))
			cmd.Dir = src
			cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+src, "XO_BIN_CACHE=off")
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("xo %v: %v\n%s", args, err, b)
			}
			if v, err := exec.Command("dwarfdump", "--verify", bin+".dSYM").CombinedOutput(); err != nil || !strings.Contains(string(v), "No errors.") {
				t.Errorf("dwarfdump --verify: %v\n%s", err, v)
			}
			largs := []string{"-b", "-o", "breakpoint set -f main.xo -l 19", "-o", "run", "-o", "frame select 1"}
			for _, x := range c.cmds {
				largs = append(largs, "-o", x)
			}
			b, err := lldb(t, true, append(largs, bin)...)
			if err != nil {
				t.Fatalf("lldb: %v\n%s", err, b)
			}
			for _, want := range c.wants {
				if !bytes.Contains(b, []byte(want)) {
					t.Errorf("lldb lacks %q:\n%s", want, b)
				}
			}
			for _, not := range c.nots {
				if bytes.Contains(b, []byte(not)) {
					t.Errorf("lldb shows %q:\n%s", not, b)
				}
			}
		})
	}
}

// TestDebugLoclistsBudget pins the size of the arm64 backend's variable
// locations (decision 0252 stage 4): the dSYM of internal/llvm's
// http_server golden has a __debug_loclists under 3 MB (2.35 MB on
// 2026-10-17, 445 KB before stage 4 described Str, Map, and struct
// values; 4.2 MB before they were compacted: one piece per run of absent
// words or of consecutive spill slots, offset pairs from a base address).
// A change that grows it past the bound should compact it again, not
// raise the bound without a measurement.
func TestDebugLoclistsBudget(t *testing.T) {
	if runtime.GOOS != "darwin" || !native() {
		t.Skip("darwin/arm64 only")
	}
	if testing.Short() {
		t.Skip("builds a program")
	}
	if _, err := exec.LookPath("otool"); err != nil {
		t.Skip("no otool")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "internal", "llvm", "testdata", "golden", "http_server")
	bin := filepath.Join(t.TempDir(), "hs")
	cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "-o", bin, "."))
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+src, "XO_BIN_CACHE=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xo build: %v\n%s", err, b)
	}
	lc, err := exec.Command("otool", "-l", filepath.Join(bin+".dSYM", "Contents", "Resources", "DWARF", "hs")).Output()
	if err != nil {
		t.Fatalf("otool -l: %v", err)
	}
	size := int64(-1)
	in := false
	for _, l := range strings.Split(string(lc), "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && f[0] == "sectname" {
			in = f[1] == "__debug_loclists"
		}
		if in && len(f) == 2 && f[0] == "size" {
			size, err = strconv.ParseInt(strings.TrimPrefix(f[1], "0x"), 16, 64)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if size < 0 {
		t.Fatalf("no __debug_loclists in the dSYM:\n%s", lc)
	}
	const budget = 3 << 20
	t.Logf("__debug_loclists: %d bytes (budget %d)", size, budget)
	if size > budget {
		t.Errorf("__debug_loclists is %d bytes, over its %d byte budget: compact the location lists (dwvars.xo dv_field, debug.xo)", size, budget)
	}
}

// TestNoLeafWithPads fails when the arm64 backend emits a leaf function
// with fault stubs and landing pads (the `_leafpads` label emit.xo puts
// after such a function's code): it keeps one unwind region, whose body
// a compact encoding then gets wrong
// (gotchas/arm64-compact-unwind-leaf-fault-frames.md); none existed on
// 2026-10-17, so the fix waits for one to test it on.
func TestNoLeafWithPads(t *testing.T) {
	if testing.Short() {
		t.Skip("generates every golden's assembly")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := filepath.Glob(filepath.Join(root, "internal", "llvm", "testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	for _, dir := range dirs {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			cmd := testtmp.Guard(exec.Command(xo, "asm", "--backend=arm64", "."))
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+dir)
			out, err := cmd.Output()
			if err != nil {
				t.Skipf("xo asm: %v", err)
			}
			if bytes.Contains(out, []byte("_leafpads:")) {
				t.Errorf("a leaf with fault stubs and landing pads: give its stubs an unwind region of their own (gotchas/arm64-compact-unwind-leaf-fault-frames.md)")
			}
		})
	}
}
