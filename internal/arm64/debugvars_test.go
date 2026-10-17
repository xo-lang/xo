package arm64

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
