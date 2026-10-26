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
// --release --debug-info keeps the DWARF, in the executable (decision
// 0346 stage 3).
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
			// the DWARF is in the executable (decision 0346 stage 3),
			// never in a bundle without --dsym
			if _, serr := os.Stat(out + ".dSYM"); serr == nil {
				t.Errorf("a build without --dsym wrote %s.dSYM", out)
			}
			lc, err := exec.Command("otool", "-l", out).Output()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(lc), "__DWARF") != c.dwarf {
				t.Errorf("__DWARF segment present: %v, want %v", !c.dwarf, c.dwarf)
			}
			info, _ := exec.Command("dwarfdump", "--debug-info", out).CombinedOutput()
			if strings.Contains(string(info), "DW_TAG_compile_unit") != c.dwarf {
				t.Errorf("DWARF compile units in the executable: %v, want %v", !c.dwarf, c.dwarf)
			}
		})
	}
}

// TestDebugInfoSameOutputLLVM stands in for the same code gate on the
// LLVM backend, whose code can differ with debug information (decision
// 0252: dbg.value calls move register allocation and the outliner): every
// golden program built with debug information runs with the output of
// its build without: o2Goldens (and every program under
// XO_TEST_LLVM_OPT) as release builds with and without --debug-info,
// the rest at -O1 with no LTO (goldenEnv) stripped (-s) and not
// (2026-10-18: two -O2 LTO builds of every golden were the largest part
// of internal/llvm's 55 minute run).
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
			skipUnselected(t, dir)
			t.Parallel()
			var runs [2]runResult
			// release builds (-O2, decision 0220) without and with
			// --debug-info for o2Goldens; the rest at goldenEnv's level
			// stripped (-s), whose output must be want.txt: the build
			// with debug information is TestGolden's, which checks
			// want.txt already (lead, 2026-10-26: building it again here
			// cost a front end and a run of every golden in the full tier)
			pair := [][]string{{"-s"}}
			env := goldenEnv(dir)
			release := o2Goldens[filepath.Base(dir)] || os.Getenv("XO_TEST_LLVM_OPT") != ""
			if release {
				pair = [][]string{{"--release"}, {"--release", "--debug-info"}}
				env = nil
			}
			for i, flags := range pair {
				res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendLLVM, Flags: flags, Env: env})
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
			if !release {
				want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
				if err != nil {
					t.Fatalf("missing want.txt: %v", err)
				}
				if runs[0].String() != string(want) {
					t.Errorf("the output differs without debug information (-s) from want.txt, which the build with it (TestGolden) prints\n--- -s\n%s\n--- want.txt\n%s", runs[0], want)
				}
				return
			}
			if runs[0] != runs[1] {
				t.Errorf("the output differs with debug information\n--- --release\n%s\n--- --release --debug-info\n%s", runs[0], runs[1])
			}
		})
	}
}
