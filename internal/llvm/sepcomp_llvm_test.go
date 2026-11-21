package llvm

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestSepcompLLVM is decision 0738's opt in: with XO_SEPCOMP_LLVM=1 a
// program built by the LLVM backend compiles its own functions, generic
// instances and clones and links std's own LLVM objects (compiler/xo
// stdcache.xo std_variant_llvm, built into the backend cache once per
// concurrency variant). Each golden here prints what its want.txt holds,
// and its build says it took the objects (`cache std llvm`): a program
// with no task (arith), one that needs the concurrent variant
// (conc_channels), a std module's generic instances (json_text), a
// fault raised in std (list_swap_guard) and the TLS and HTTP stack
// (http_tls), whose whole program build spends 30 s and more in clang.
// XO_SEPCOMP is passed empty: a tier sets it to 0 for every build of
// the test process (xotool, XO_TEST_SEPCOMP=0), and the opt in holds
// with it unset or empty.
func TestSepcompLLVM(t *testing.T) {
	if testing.Short() {
		t.Skip("builds std's LLVM objects (about 40 s a variant) and five programs")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("std's variants are built for darwin, linux and windows hosts; the LLVM objects are tested on darwin and linux")
	}
	// the objects are kept by clang and by the optimization level
	// (XO_LLVM_OPT) too, which std's variant key does not hold: a build
	// at another level must not link the default's objects
	t.Run("opt level", func(t *testing.T) {
		dir := filepath.Join("testdata", "golden", "arith")
		opts := xotool.Options{Backend: xotool.BackendLLVM, Flags: []string{"--phase-times"}, Env: []string{"XO_SEPCOMP_LLVM=1", "XO_SEPCOMP=", "XO_WHOLE_BUILD=off", "XO_BIN_CACHE=off"}}
		_, o2 := buildRunLeak(t, dir, opts)
		opts.Env = append(opts.Env, "XO_LLVM_OPT=O1")
		res, o1 := buildRunLeak(t, dir, opts)
		a, b := stdLLVMIDs(o2.Output), stdLLVMIDs(o1.Output)
		if len(a) != 1 || len(b) != 1 || a[0] == b[0] {
			t.Fatalf("std's LLVM objects at the default level %v and at XO_LLVM_OPT=O1 %v: want one directory each, not the same\n%s", a, b, o1.Output)
		}
		want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != res.String() {
			t.Errorf("the O1 build differs from want.txt\n--- want\n%s\n--- got\n%s", want, res)
		}
	})
	for _, name := range []string{"arith", "conc_channels", "json_text", "list_swap_guard", "http_tls"} {
		dir := filepath.Join("testdata", "golden", name)
		t.Run(name, func(t *testing.T) {
			res, build := buildRunLeak(t, dir, xotool.Options{Backend: xotool.BackendLLVM, Flags: []string{"--phase-times"}, Env: []string{"XO_SEPCOMP_LLVM=1", "XO_SEPCOMP=", "XO_WHOLE_BUILD=off", "XO_BIN_CACHE=off"}})
			if len(stdLLVMIDs(build.Output)) != 1 || !strings.Contains(build.Output, " conc=false") || !strings.Contains(build.Output, " conc=true") {
				t.Fatalf("the build did not take std's LLVM objects (no `cache std llvm` lines):\n%s", build.Output)
			}
			if strings.Contains(build.Output, "not built") {
				t.Fatalf("std's LLVM objects were not built:\n%s", build.Output)
			}
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != res.String() {
				t.Errorf("the separately compiled build differs from want.txt\n--- want\n%s\n--- got\n%s", want, res)
			}
		})
	}
}

// stdLLVMIDs is the distinct ids of the `cache std llvm ID conc=...`
// lines of a build's --phase-times output (stdcache.xo std_variant_llvm:
// the id names clang and the optimization level).
func stdLLVMIDs(out string) []string {
	var ids []string
	seen := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) >= 5 && f[0] == "cache" && f[1] == "std" && f[2] == "llvm" && strings.HasPrefix(f[4], "conc=") && !seen[f[3]] {
			seen[f[3]] = true
			ids = append(ids, f[3])
		}
	}
	return ids
}
