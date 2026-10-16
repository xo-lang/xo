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
