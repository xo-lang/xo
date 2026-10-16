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

// TestDebugLineDarwin checks the DWARF line tables of decision 0239 on
// darwin/arm64: a build writes PROG.dSYM next to the executable, whose
// line table maps an address of main to main.xo and the line of its
// statement; `-s` writes none (and removes no executable bytes: darwin
// keeps DWARF out of the executable). lldb's `b main.xo:9` resolving
// there is checked by hand (knowledgebase/infrastructure/toolchain.md).
func TestDebugLineDarwin(t *testing.T) {
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
	out, err := testtmp.Dir("xo-debugline-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(out)
	src, err := filepath.Abs(filepath.Join("testdata", "debugline"))
	if err != nil {
		t.Fatal(err)
	}
	build := func(name string, extra ...string) string {
		bin := filepath.Join(out, name)
		args := append([]string{"build", "--backend=arm64"}, extra...)
		args = append(args, "-o", bin, ".")
		cmd := exec.Command(xo, args...)
		cmd.Dir = src
		cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+src, "XO_BIN_CACHE=off")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo %v: %v\n%s", args, err, b)
		}
		return bin
	}
	bin := build("p")
	dsym := bin + ".dSYM"
	if _, err := os.Stat(filepath.Join(dsym, "Contents", "Resources", "DWARF", "p")); err != nil {
		t.Fatalf("no dSYM: %v", err)
	}
	info, err := exec.Command("dwarfdump", "--debug-info", dsym).CombinedOutput()
	if err != nil {
		t.Fatalf("dwarfdump: %v\n%s", err, info)
	}
	m := regexp.MustCompile(`DW_AT_name\s+\("main"\)\s+DW_AT_low_pc\s+\((0x[0-9a-f]+)\)`).FindSubmatch(info)
	if m == nil {
		t.Fatalf("no subprogram main in\n%s", info)
	}
	// main's first statement, `var t = 0`, is line 7
	look, err := exec.Command("dwarfdump", "--lookup="+string(m[1]), dsym).CombinedOutput()
	if err != nil {
		t.Fatalf("dwarfdump --lookup: %v\n%s", err, look)
	}
	if !strings.Contains(string(look), "file 'main.xo', line 7") && !strings.Contains(string(look), "file 'main.xo', line 8") {
		t.Errorf("main's address is not at main.xo:7 or 8:\n%s", look)
	}
	if v, err := exec.Command("dwarfdump", "--verify", dsym).CombinedOutput(); err != nil || !strings.Contains(string(v), "No errors.") {
		t.Errorf("dwarfdump --verify: %v\n%s", err, v)
	}
	stripped := build("q", "-s")
	if _, err := os.Stat(stripped + ".dSYM"); err == nil {
		t.Errorf("-s wrote %s.dSYM", stripped)
	}
}

// TestDebugLineDarwinCC checks the dSYM of the darwin links that do not go
// through selfhost/macho (decision 0239): the LLVM backend's and arm64
// with --ld=cc, which run dsymutil after the cc link. Each must write
// PROG.dSYM whose line table names main.xo and verifies, and lldb must
// resolve a breakpoint at main.xo:9 from it (no run: the line tables
// alone). A second build from the binary cache must bring the dSYM back
// (the cache keeps its DWARF beside the entry), for the macho link too.
func TestDebugLineDarwinCC(t *testing.T) {
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
	out, err := testtmp.Dir("xo-debugline-cc-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(out)
	src, err := filepath.Abs(filepath.Join("testdata", "debugline"))
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(out, "bincache")
	build := func(name string, args ...string) string {
		bin := filepath.Join(out, name)
		args = append([]string{"build"}, args...)
		args = append(args, "-o", bin, ".")
		cmd := exec.Command(xo, args...)
		cmd.Dir = src
		cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+src, "XO_BIN_CACHE="+cache)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo %v: %v\n%s", args, err, b)
		}
		return bin
	}
	check := func(bin string) {
		t.Helper()
		dsym := bin + ".dSYM"
		lines, err := exec.Command("dwarfdump", "--debug-line", dsym).CombinedOutput()
		if err != nil || !strings.Contains(string(lines), "main.xo") {
			t.Fatalf("dwarfdump --debug-line %s: %v\n%.2000s", dsym, err, lines)
		}
		if v, err := exec.Command("dwarfdump", "--verify", dsym).CombinedOutput(); err != nil || !strings.Contains(string(v), "No errors.") {
			t.Errorf("dwarfdump --verify %s: %v\n%s", dsym, err, v)
		}
		b, err := exec.Command("lldb", "-b", "-o", "breakpoint set -f main.xo -l 9", bin).CombinedOutput()
		if err != nil || !regexp.MustCompile(`Breakpoint 1: where = .* at main\.xo:\d+`).Match(b) {
			t.Errorf("lldb b main.xo:9 on %s: %v\n%s", bin, err, b)
		}
	}
	for _, c := range []struct {
		name string
		args []string
	}{
		{"llvm", []string{"--backend=llvm"}},
		{"cc", []string{"--backend=arm64", "--ld=cc"}},
		{"macho", []string{"--backend=arm64", "--ld=macho"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			bin := build(c.name, c.args...)
			check(bin)
			// from the binary cache: the executable and its dSYM
			if err := os.RemoveAll(bin + ".dSYM"); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(bin); err != nil {
				t.Fatal(err)
			}
			build(c.name, c.args...)
			if _, err := os.Stat(bin + ".dSYM"); err != nil {
				t.Fatalf("no dSYM after a cache hit: %v", err)
			}
			check(bin)
		})
	}
}

// TestDebugLineInlined checks the inlined frames of decision 0239 on
// darwin/arm64: the arm64 inliner copies testdata/inlined's scale into
// main, and the dSYM describes the copy with a DW_TAG_inlined_subroutine
// whose abstract origin is scale and whose call line is main's call (12),
// so an address in the copy looks up as scale at its own lines (5 or 6),
// and lldb stopped at `b scale` shows scale as an inlined frame above
// main at line 12.
func TestDebugLineInlined(t *testing.T) {
	if runtime.GOOS != "darwin" || !native() {
		t.Skip("darwin/arm64 only")
	}
	if testing.Short() {
		t.Skip("builds a program")
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
	out, err := testtmp.Dir("xo-inlined-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(out)
	src, err := filepath.Abs(filepath.Join("testdata", "inlined"))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(out, "p")
	cmd := exec.Command(xo, "build", "--backend=arm64", "--ld=macho", "-o", bin, ".")
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+src, "XO_BIN_CACHE=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xo build: %v\n%s", err, b)
	}
	dsym := bin + ".dSYM"
	info, err := exec.Command("dwarfdump", "--debug-info", dsym).CombinedOutput()
	if err != nil {
		t.Fatalf("dwarfdump: %v\n%s", err, info)
	}
	m := regexp.MustCompile(`DW_TAG_inlined_subroutine\s+DW_AT_abstract_origin\s+\(0x[0-9a-f]+ "scale"\)\s+DW_AT_low_pc\s+\((0x[0-9a-f]+)\)[\s\S]*?DW_AT_call_line\s+\((\d+)\)`).FindSubmatch(info)
	if m == nil {
		t.Fatalf("no inlined subroutine of scale in\n%s", info)
	}
	if string(m[2]) != "12" {
		t.Errorf("scale's inlined copy has call line %s, want 12", m[2])
	}
	look, err := exec.Command("dwarfdump", "--lookup="+string(m[1]), dsym).CombinedOutput()
	if err != nil {
		t.Fatalf("dwarfdump --lookup: %v\n%s", err, look)
	}
	if !strings.Contains(string(look), "DW_TAG_inlined_subroutine") || !regexp.MustCompile(`file '[^']*main\.xo', line [56]\b`).Match(look) {
		t.Errorf("the inlined copy's address does not look up as scale's lines:\n%s", look)
	}
	if v, err := exec.Command("dwarfdump", "--verify", dsym).CombinedOutput(); err != nil || !strings.Contains(string(v), "No errors.") {
		t.Errorf("dwarfdump --verify: %v\n%s", err, v)
	}
	bt, err := exec.Command("lldb", "-b", "-o", "b scale", "-o", "run", "-o", "bt 4", bin).CombinedOutput()
	if err != nil {
		t.Fatalf("lldb: %v\n%s", err, bt)
	}
	if !regexp.MustCompile(`frame #0: .*scale at main\.xo:[56]:\d+ .*\[inlined\]`).Match(bt) || !regexp.MustCompile(`frame #1: .*main at main\.xo:12:\d+`).Match(bt) {
		t.Errorf("lldb's backtrace lacks the inlined frame of scale above main at line 12:\n%s", bt)
	}
}
