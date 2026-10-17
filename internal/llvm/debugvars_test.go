package llvm

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestStrippedNoDWARF checks that the LLVM builds without debug
// information have none (decision 0252): -s and --release leave no
// dSYM, no __DWARF segment, and no compile unit in the executable, and
// --release --debug-info keeps the dSYM.
func TestStrippedNoDWARF(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only (Linux: checked with llvm-readelf by hand)")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	for _, tool := range []string{"otool", "dwarfdump"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	dir := filepath.Join("testdata", "golden", "arith")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("no %s: %v", dir, err)
	}
	for _, c := range []struct {
		name  string
		flags []string
		dwarf bool
	}{
		{"strip", []string{"-s"}, false},
		{"release", []string{"--release"}, false},
		{"release-debug", []string{"--release", "--debug-info"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), c.name)
			res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendLLVM, Flags: c.flags, Out: out, Env: []string{"XO_BIN_CACHE=off"}})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if res.Failed {
				t.Fatalf("build failed:\n%s", res.Output)
			}
			_, serr := os.Stat(out + ".dSYM")
			if (serr == nil) != c.dwarf {
				t.Errorf("dSYM present: %v, want %v", serr == nil, c.dwarf)
			}
			lc, err := exec.Command("otool", "-l", out).Output()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(lc), "__DWARF") {
				t.Errorf("the executable has a __DWARF segment")
			}
			info, _ := exec.Command("dwarfdump", "--debug-info", out).CombinedOutput()
			if strings.Contains(string(info), "DW_TAG_compile_unit") {
				t.Errorf("the executable has DWARF compile units")
			}
		})
	}
}

// TestDebugInfoSameOutputLLVM stands in for the same code gate on the
// LLVM backend, whose code can differ with debug information (decision
// 0252: dbg.value calls move register allocation and the outliner): every
// golden program built with --release --debug-info runs with the output of
// its --release build.
func TestDebugInfoSameOutputLLVM(t *testing.T) {
	if testing.Short() {
		t.Skip("builds every golden program twice")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	for _, dir := range dirs {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			var runs [2]runResult
			for i, flags := range [][]string{{"--release"}, {"--release", "--debug-info"}} {
				res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendLLVM, Flags: flags})
				if err != nil {
					t.Fatalf("build %v: %v", flags, err)
				}
				if res.Dir != "" {
					defer os.RemoveAll(res.Dir)
				}
				if res.Failed {
					if strings.Contains(res.Output, "XO1504") {
						t.Skipf("unsupported: %s", firstLine(res.Output))
					}
					t.Fatalf("build %v failed:\n%s", flags, res.Output)
				}
				r, err := runBinary(res.Binaries[xotool.Host()], dir)
				if err != nil {
					t.Fatalf("run %v: %v", flags, err)
				}
				runs[i] = r
			}
			if runs[0] != runs[1] {
				t.Errorf("the output differs with debug information\n--- --release\n%s\n--- --release --debug-info\n%s", runs[0], runs[1])
			}
		})
	}
}
