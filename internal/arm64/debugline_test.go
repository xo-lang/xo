package arm64

import (
	"context"
	"debug/macho"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestDebugLineDarwin checks the DWARF line tables of decision 0239 on
// darwin/arm64: a build embeds them in the executable's __DWARF segment
// and writes no PROG.dSYM (decision 0346); the line table maps an
// address of main to main.xo and the line of its statement. `--dsym`
// writes the bundle too, with the same tables; `-s` writes neither.
// lldb's `b main.xo:9` resolving there is checked by hand
// (knowledgebase/infrastructure/toolchain.md).
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
	// buildIn builds src from the directory wd
	buildIn := func(wd, name string, extra ...string) string {
		bin := filepath.Join(out, name)
		args := append([]string{"build", "--backend=arm64"}, extra...)
		args = append(args, "-o", bin, src)
		cmd := testtmp.Guard(exec.Command(xo, args...))
		cmd.Dir = wd
		cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+wd, "XO_BIN_CACHE=off")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo %v: %v\n%s", args, err, b)
		}
		return bin
	}
	build := func(name string, extra ...string) string { return buildIn(src, name, extra...) }
	bin := build("p")
	if _, err := os.Stat(bin + ".dSYM"); err == nil {
		t.Errorf("a build without --dsym wrote %s.dSYM", bin)
	}
	if !hasDwarfSegment(t, bin) {
		t.Fatalf("%s has no __DWARF segment", bin)
	}
	dsym := bin
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
	// atos names the program's first function, which a part's text
	// marker (xo_utext_) shared its address with (2026-10-22)
	if _, err := exec.LookPath("atos"); err == nil {
		syms, err := exec.Command("nm", bin).CombinedOutput()
		if err != nil {
			t.Fatalf("nm: %v\n%s", err, syms)
		}
		a := regexp.MustCompile(`(?m)^([0-9a-f]+) t _xo_f_main$`).FindSubmatch(syms)
		if a == nil {
			t.Fatalf("no _xo_f_main in\n%s", syms)
		}
		if strings.Contains(string(syms), "_xo_utext_") {
			t.Errorf("the symbol table has a part's text marker:\n%s", syms)
		}
		at, err := exec.Command("atos", "-o", bin, "0x"+string(a[1])).CombinedOutput()
		if err != nil || !strings.Contains(string(at), "(main.xo:") {
			t.Errorf("atos -o %s 0x%s: %v\n%s", bin, a[1], err, at)
		}
	}
	// a build from another directory: DW_AT_comp_dir is the module's
	// directory (the sources' names are relative to it), not the working
	// directory, where lldb looked for main.xo (2026-10-22)
	other := buildIn(out, "o")
	ci, err := exec.Command("dwarfdump", "--debug-info", other).CombinedOutput()
	if err != nil {
		t.Fatalf("dwarfdump: %v\n%s", err, ci)
	}
	cd := regexp.MustCompile(`DW_AT_comp_dir\s+\("([^"]*)"\)`).FindSubmatch(ci)
	if cd == nil {
		t.Fatalf("no DW_AT_comp_dir in\n%s", ci)
	}
	want, _ := filepath.EvalSymlinks(src)
	if got, _ := filepath.EvalSymlinks(string(cd[1])); got != want {
		t.Errorf("a build from %s: DW_AT_comp_dir %s, want %s", out, cd[1], src)
	}
	withDsym := build("d", "--dsym")
	d := filepath.Join(withDsym+".dSYM", "Contents", "Resources", "DWARF", "d")
	if v, err := exec.Command("dwarfdump", "--verify", d).CombinedOutput(); err != nil || !strings.Contains(string(v), "No errors.") {
		t.Errorf("--dsym: dwarfdump --verify %s: %v\n%s", d, err, v)
	}
	stripped := build("q", "-s", "--dsym")
	if _, err := os.Stat(stripped + ".dSYM"); err == nil {
		t.Errorf("-s wrote %s.dSYM", stripped)
	}
	if hasDwarfSegment(t, stripped) {
		t.Errorf("-s left a __DWARF segment in %s", stripped)
	}
}

// hasDwarfSegment reports whether the Mach-O executable bin has a
// __DWARF segment (decision 0346).
func hasDwarfSegment(t *testing.T, bin string) bool {
	t.Helper()
	f, err := macho.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return f.Segment("__DWARF") != nil
}

// dwarfFile is where bin's DWARF is: its dSYM's DWARF file when the link
// wrote one (cc and the LLVM backend through dsymutil, or --dsym), else
// bin itself (compiler/macho embeds it, decision 0346).
func dwarfFile(bin string) string {
	d := filepath.Join(bin+".dSYM", "Contents", "Resources", "DWARF", filepath.Base(bin))
	if _, err := os.Stat(d); err == nil {
		return d
	}
	return bin
}

// TestDebugLineDarwinCC checks the DWARF of the darwin links that do not
// go through compiler/macho (decision 0239): the LLVM backend's and arm64
// with --ld=cc, which run dsymutil after the cc link and then move the
// dSYM's DWARF into the executable (decision 0346 stage 3), as the macho
// link writes it. Every executable must carry a __DWARF segment that
// verifies and a signature codesign accepts; its DWARF (the bundle's with
// --dsym, which alone keeps PROG.dSYM) must name main.xo and verify, and
// lldb must resolve a breakpoint at main.xo:9 from it (no run: the line
// tables alone). A second build from the binary cache must give the same
// (the cache keeps a bundle's DWARF beside the entry).
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
		cmd := testtmp.Guard(exec.Command(xo, args...))
		cmd.Dir = src
		cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+src, "XO_BIN_CACHE="+cache)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo %v: %v\n%s", args, err, b)
		}
		return bin
	}
	check := func(bin string) {
		t.Helper()
		if !hasDwarfSegment(t, bin) {
			t.Errorf("%s has no __DWARF segment", bin)
		}
		if v, err := exec.Command("dwarfdump", "--verify", bin).CombinedOutput(); err != nil || !strings.Contains(string(v), "No errors.") {
			t.Errorf("dwarfdump --verify %s: %v\n%s", bin, err, v)
		}
		if v, err := exec.Command("codesign", "--verify", "--strict", bin).CombinedOutput(); err != nil {
			t.Errorf("codesign --verify --strict %s: %v\n%s", bin, err, v)
		}
		dsym := dwarfFile(bin)
		lines, err := exec.Command("dwarfdump", "--debug-line", dsym).CombinedOutput()
		if err != nil || !strings.Contains(string(lines), "main.xo") {
			t.Fatalf("dwarfdump --debug-line %s: %v\n%.2000s", dsym, err, lines)
		}
		if v, err := exec.Command("dwarfdump", "--verify", dsym).CombinedOutput(); err != nil || !strings.Contains(string(v), "No errors.") {
			t.Errorf("dwarfdump --verify %s: %v\n%s", dsym, err, v)
		}
		b, err := lldb(t, false, "-b", "-o", "breakpoint set -f main.xo -l 9", bin)
		if err != nil || !regexp.MustCompile(`Breakpoint 1: where = .* at main\.xo:\d+`).Match(b) {
			t.Errorf("lldb b main.xo:9 on %s: %v\n%s", bin, err, b)
		}
	}
	for _, c := range []struct {
		name   string
		args   []string
		bundle bool
	}{
		// ld64's links carry the dSYM's DWARF too (decision 0346 stage
		// 3), the bundle only for --dsym
		{"llvm", []string{"--backend=llvm"}, false},
		{"llvm-dsym", []string{"--backend=llvm", "--dsym"}, true},
		{"cc", []string{"--backend=arm64", "--ld=cc"}, false},
		{"cc-dsym", []string{"--backend=arm64", "--ld=cc", "--dsym"}, true},
		{"macho", []string{"--backend=arm64", "--ld=macho"}, false},
		{"macho-dsym", []string{"--backend=arm64", "--ld=macho", "--dsym"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			bin := build(c.name, c.args...)
			if _, err := os.Stat(bin + ".dSYM"); (err == nil) != c.bundle {
				t.Fatalf("%s.dSYM present: %v, want %v", bin, err == nil, c.bundle)
			}
			check(bin)
			// from the binary cache: the executable and its dSYM
			if err := os.RemoveAll(bin + ".dSYM"); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(bin); err != nil {
				t.Fatal(err)
			}
			build(c.name, c.args...)
			if _, err := os.Stat(bin + ".dSYM"); (err == nil) != c.bundle {
				t.Fatalf("%s.dSYM present after a cache hit: %v, want %v", bin, err == nil, c.bundle)
			}
			check(bin)
		})
	}
}

// TestDebugLineInlined checks the inlined frames of decision 0239 on
// darwin/arm64: the arm64 inliner copies testdata/inlined's scale into
// main, and the DWARF describes the copy with a DW_TAG_inlined_subroutine
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
	cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "--ld=macho", "-o", bin, "."))
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+src, "XO_BIN_CACHE=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xo build: %v\n%s", err, b)
	}
	dsym := dwarfFile(bin)
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
	// a frame's parameters follow its name when it has any with debug
	// information (main's os, 2026-10-22: "main(os=<unavailable>) at")
	bt, err := lldb(t, true, "-b", "-o", "b scale", "-o", "run", "-o", "bt 4", bin)
	if err != nil {
		t.Fatalf("lldb: %v\n%s", err, bt)
	}
	if !regexp.MustCompile(`frame #0: .*scale(\(.*\))? at main\.xo:[56]:\d+ .*\[inlined\]`).Match(bt) || !regexp.MustCompile(`frame #1: .*main(\(.*\))? at main\.xo:12:\d+`).Match(bt) {
		t.Errorf("lldb's backtrace lacks the inlined frame of scale above main at line 12:\n%s", bt)
	}
}

// lldb runs lldb with args under a 60 s limit, its output and error. A
// run (attaching to the program) needs macOS's Developer Mode: with it
// off (`DevToolsSecurity -status` says disabled) lldb waits for an
// authorization prompt forever, so the test skips; it also skips when
// the limit is hit, rather than hang the package's run.
func lldb(t *testing.T, run bool, args ...string) ([]byte, error) {
	t.Helper()
	if run && devToolsDisabled() {
		t.Skip("lldb cannot attach: Developer Mode is off (DevToolsSecurity -status: disabled; enable it with `sudo DevToolsSecurity -enable`)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := testtmp.Guard(exec.CommandContext(ctx, "lldb", args...))
	cmd.WaitDelay = 5 * time.Second
	b, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Skipf("lldb did not finish in 60 s (Developer Mode off, or an authorization prompt waiting?):\n%s", b)
	}
	return b, err
}

// devToolsDisabled reports whether macOS's Developer Mode is off, which
// keeps lldb from attaching without a prompt.
func devToolsDisabled() bool {
	b, err := exec.Command("DevToolsSecurity", "-status").CombinedOutput()
	return err == nil && strings.Contains(string(b), "disabled")
}
