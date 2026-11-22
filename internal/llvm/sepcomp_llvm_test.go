package llvm

import (
	"crypto/sha256"
	"debug/elf"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	// the backend cache on by name, in a directory of this test's own: a
	// tier and a gate export XO_BACKEND_CACHE=off, under which no std
	// objects are kept and the whole program is built (tier 924 failed
	// here: `cache std off`, task #656); and the test's std objects stay
	// out of the host's cache
	sepEnv := []string{"XO_SEPCOMP_LLVM=1", "XO_SEPCOMP=", "XO_WHOLE_BUILD=off", "XO_BIN_CACHE=off", "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR=" + t.TempDir()}
	t.Run("opt level", func(t *testing.T) {
		dir := filepath.Join("testdata", "golden", "arith")
		opts := xotool.Options{Backend: xotool.BackendLLVM, Flags: []string{"--phase-times"}, Env: sepEnv}
		_, o2 := buildRunLeak(t, dir, opts)
		opts.Env = append(append([]string{}, sepEnv...), "XO_LLVM_OPT=O1")
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
	// on ELF std's modules are optimized across each other when the
	// objects are made (distributed ThinLTO, which needs lld): the build
	// says which objects it linked. Without that step a program ran 1.5
	// times the whole program's time
	t.Run("std modules optimized together", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("ELF only: darwin links the plain objects")
		}
		cl, err := xotool.Clang()
		if err != nil {
			t.Skip("clang not found")
		}
		out, err := exec.Command(cl, "-print-prog-name=ld.lld").Output()
		if _, serr := os.Stat(strings.TrimSpace(string(out))); err != nil || serr != nil {
			t.Skip("no lld: the plain objects are the fallback")
		}
		_, build := buildRunLeak(t, filepath.Join("testdata", "golden", "arith"), xotool.Options{Backend: xotool.BackendLLVM, Flags: []string{"--phase-times"}, Env: sepEnv})
		if !strings.Contains(build.Output, "(thinlto)") || strings.Contains(build.Output, "(plain)") {
			t.Fatalf("std's LLVM objects are not the ThinLTO ones (want `(thinlto)` on both `cache std llvm` lines):\n%s", build.Output)
		}
		// and the program's own module took std's small functions from
		// std's bitcode, kept beside those objects (the program to std
		// half: a call into std was a call whatever its size, atomic add
		// 1.47 times the whole program's time). The goldens below then
		// run with the imports: one read a std string constant that
		// std's objects did not define under the name the import gave
		// it (`undefined hidden symbol: .rs.6211.llvm...`)
		if !strings.Contains(build.Output, "cache std llvm imports on") {
			t.Fatalf("the program's module was compiled without std's imports (want `cache std llvm imports on`):\n%s", build.Output)
		}
	})
	// two builds of std's objects, each in a cache of its own, hold the
	// same code and the same symbols. They did not (task #607): a ThinLTO
	// module is identified by the path clang was given, the modules sat
	// in a randomly named directory, and the names of promoted locals
	// and the import order follow that path; one build in three then
	// gave a fault report whose traceback named every frame `?`. The
	// objects' bytes differ by design (each names its .dwo file in its
	// cache), so the executable sections and the symbol names are
	// compared
	t.Run("std objects reproducible", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("ELF only: the ThinLTO objects")
		}
		dir := filepath.Join("testdata", "golden", "arith")
		// the cache on by name: a gate exports XO_BACKEND_CACHE=off, and
		// std's variants are kept only in a cache that is on
		// and clang itself, not the test binary's clang cache
		// (xotool.EnableTestClangCache): that answers a module's compile
		// by its content, so the second build would get the first's
		// bitcode, path and all, and the two would agree whatever xo did
		env := []string{"XO_SEPCOMP_LLVM=1", "XO_SEPCOMP=", "XO_WHOLE_BUILD=off", "XO_BIN_CACHE=off", "XO_BACKEND_CACHE=on"}
		if real := os.Getenv("XO_CLANG_CACHED"); real != "" {
			env = append(env, "XO_CLANG="+real)
		}
		var sums [2]map[string]string
		for i := range sums {
			cache := t.TempDir()
			_, build := buildRunLeak(t, dir, xotool.Options{Backend: xotool.BackendLLVM, Flags: []string{"--phase-times"}, Env: append(append([]string{}, env...), "XO_BACKEND_CACHE_DIR="+cache)})
			objs, _ := filepath.Glob(filepath.Join(cache, "std", "*.llvm-*", "std.*.o"))
			if len(objs) == 0 {
				t.Fatalf("no std objects under %s:\n%s", cache, build.Output)
			}
			sums[i] = map[string]string{}
			for _, o := range objs {
				rel, _ := filepath.Rel(filepath.Join(cache, "std"), o)
				sums[i][rel] = elfCodeSum(t, o)
			}
		}
		if len(sums[0]) != len(sums[1]) {
			t.Fatalf("%d objects in the first build, %d in the second", len(sums[0]), len(sums[1]))
		}
		for rel, a := range sums[0] {
			if b := sums[1][rel]; a != b {
				t.Errorf("%s: code and symbols differ between two builds (%s, %s)", rel, a, b)
			}
		}
	})
	for _, name := range []string{"arith", "conc_channels", "json_text", "list_swap_guard", "http_tls"} {
		dir := filepath.Join("testdata", "golden", name)
		t.Run(name, func(t *testing.T) {
			res, build := buildRunLeak(t, dir, xotool.Options{Backend: xotool.BackendLLVM, Flags: []string{"--phase-times"}, Env: sepEnv})
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

// elfCodeSum hashes an ELF object's executable sections (name and
// bytes) and its symbol names, in the file's order: what two builds of
// the same sources must agree on, whatever paths their debug sections
// hold.
func elfCodeSum(t *testing.T, path string) string {
	t.Helper()
	f, err := elf.Open(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	defer f.Close()
	h := sha256.New()
	for _, s := range f.Sections {
		if s.Flags&elf.SHF_EXECINSTR == 0 {
			continue
		}
		data, err := s.Data()
		if err != nil {
			t.Fatalf("%s: section %s: %v", path, s.Name, err)
		}
		fmt.Fprintf(h, "section %s %d\n", s.Name, len(data))
		h.Write(data)
	}
	syms, err := f.Symbols()
	if err != nil && !errors.Is(err, elf.ErrNoSymbols) {
		t.Fatalf("%s: symbols: %v", path, err)
	}
	for _, s := range syms {
		fmt.Fprintf(h, "symbol %s\n", s.Name)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
