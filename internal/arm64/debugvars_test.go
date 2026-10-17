package arm64

import (
	"bytes"
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
// testdata/debugvars built by the arm64 backend, linked by selfhost/macho
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
			cmd := exec.Command(xo, args...)
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

// TestDwarfReader checks decision 0253's DWARF reader (selfhost/debug,
// through `xo dev dwarf`) on testdata/debugvars: from the arm64
// backend's dSYM (selfhost/macho and dsymutil) it reads sum's variables
// with the address ranges dwarfdump prints for each, and from the LLVM
// backend's, sum inlined into main with k the constant 10.
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
			cmd := exec.Command(xo, args...)
			cmd.Dir = src
			cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+src, "XO_BIN_CACHE=off")
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("xo %v: %v\n%s", args, err, b)
			}
			dw := filepath.Join(bin+".dSYM", "Contents", "Resources", "DWARF", c.name)
			got, err := exec.Command(xo, "dev", "dwarf", "--func=sum", dw).CombinedOutput()
			if err != nil {
				t.Fatalf("xo dev dwarf: %v\n%s", err, got)
			}
			for _, w := range c.want {
				if !bytes.Contains(got, []byte(w)) {
					t.Errorf("xo dev dwarf --func=sum lacks %q:\n%s", w, got)
				}
			}
			// the runtime's static words, the debugger's way to its records
			all, err := exec.Command(xo, "dev", "dwarf", dw).CombinedOutput()
			if err != nil {
				t.Fatalf("xo dev dwarf: %v\n%s", err, all)
			}
			if !regexp.MustCompile(`(?m)^global var xo_static: \[256\]Int line 0\n\s+always \[0x[0-9a-f]+\]$`).Match(all) {
				t.Errorf("xo dev dwarf: no xo_static global with an address:\n%s", lastLines(all, 20))
			}
			if c.name == "llvm" {
				return
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
