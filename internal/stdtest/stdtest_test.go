// Package stdtest runs the Xo tests of std modules (decision 0262): each
// std/.../NAME_test.xo beside its module's source is run with `xo test
// --std-module=MODULE`, so the tests have the module's private
// functions. Backends by tier: -short the host's native backend (arm64
// or amd64, decision 0270; llvm on another host); the default LLVM and
// the native one; XO_STDTEST_BACKENDS (comma separated) names them
// instead. Modules run in parallel, XO_STDTEST_JOBS at a time (default
// min(GOMAXPROCS, 4)). The large differential corpora generated from Go
// are plain test directories under testdata/corpus, which TestStdCorpora
// runs outside -short. Faults are programs rather than test files, one per
// case under testdata/faults, which TestSliceFaults builds and runs on
// every corpus backend outside -short (faults_test.go).
package stdtest

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// modules are the std modules with a test file, as --std-module names
// them (`json`, `native/http`, `native/llrt/scan`), sorted.
func modules(t *testing.T, root string) []string {
	t.Helper()
	std := filepath.Join(root, "std")
	var out []string
	err := filepath.WalkDir(std, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// std/rt is the Go backend's runtime, in Go
			if d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") || p == filepath.Join(std, "rt") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.xo") {
			return nil
		}
		src := strings.TrimSuffix(p, "_test.xo")
		if _, err := os.Stat(src + ".xo"); err != nil {
			if _, err := os.Stat(src + "_c.xo"); err != nil {
				t.Errorf("%s: no module source beside it (%s.xo)", p, filepath.Base(src))
				return nil
			}
		}
		rel, _ := filepath.Rel(std, src)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func backends() []string {
	if v := os.Getenv("XO_STDTEST_BACKENDS"); v != "" {
		return strings.Split(v, ",")
	}
	native := ""
	switch {
	case runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux"):
		native = "arm64"
	case runtime.GOARCH == "amd64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux" || runtime.GOOS == "windows"):
		native = "amd64"
	}
	if native == "" {
		return []string{"llvm"}
	}
	if testing.Short() {
		return []string{native}
	}
	return []string{"llvm", native}
}

// corporaBackends are the backends the corpora run on: backends(), and the
// Go backend where the go command exists (std/rt is its runtime, which the
// std module tests do not reach; before 2026-10-23 a -run with /go/ matched
// nothing). XO_STDTEST_BACKENDS names them exactly instead.
func corporaBackends() []string {
	bs := backends()
	if os.Getenv("XO_STDTEST_BACKENDS") != "" {
		return bs
	}
	if _, err := exec.LookPath("go"); err == nil {
		bs = append(bs, "go")
	}
	return bs
}

// nativeOnlyCorpora are the corpora whose module calls C, which the Go
// backend refuses (XO1201): the reason each skips there.
var nativeOnlyCorpora = map[string]string{
	"sqlite": "std/sqlite calls SQLite's C code, which the Go backend refuses (XO1201)",
}

// goOnlyCorpora are the corpora that call Go packages (`use go`), which
// the native backends refuse (XO1201): the reason each skips there.
var goOnlyCorpora = map[string]string{
	"gofuzz": "std/go's corpus calls Go packages (use go), which the native backends refuse (XO1201)",
}

// jobs is how many xo test processes run at once.
func jobs() int {
	if v, err := strconv.Atoi(os.Getenv("XO_STDTEST_JOBS")); err == nil && v > 0 {
		return v
	}
	return min(runtime.GOMAXPROCS(0), 4)
}

// TestStdModules runs every std module's tests on each backend.
// stdModuleArg: the --std-module argument for the module m of mods,
// std/m when std/native has a module of the same name (a bare cbor is
// ambiguous beside native/cbor: gotchas/std-module-name-ambiguous-with-native.md)
func stdModuleArg(m string, mods []string) string {
	for _, o := range mods {
		if o == "native/"+m {
			return "std/" + m
		}
	}
	return m
}

func TestStdModuleArg(t *testing.T) {
	mods := []string{"cbor", "csv", "native/cbor", "native/llrt/scan"}
	for _, c := range [][2]string{{"cbor", "std/cbor"}, {"csv", "csv"}, {"native/cbor", "native/cbor"}, {"native/llrt/scan", "native/llrt/scan"}} {
		if got := stdModuleArg(c[0], mods); got != c[1] {
			t.Errorf("stdModuleArg(%q) = %q, want %q", c[0], got, c[1])
		}
	}
}

func TestStdModules(t *testing.T) {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	mods := modules(t, root)
	if len(mods) == 0 {
		t.Skip("no std module has a _test.xo file")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	sem := make(chan struct{}, jobs())
	for _, b := range backends() {
		for _, m := range mods {
			// the runtime's modules (llrt) and the implementations run on
			// the native backends only (decision 0262 item 4)
			t.Run(b+"/"+m, func(t *testing.T) {
				t.Parallel()
				sem <- struct{}{}
				defer func() { <-sem }()
				cmd := testtmp.Guard(exec.Command(xo, "test", "--std-module="+stdModuleArg(m, mods), "--backend="+b))
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+root)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("xo test --std-module=%s --backend=%s: %v\n%s", m, b, err, out)
				}
				t.Logf("%s", out)
			})
		}
	}
}

// TestStdCorpora runs the large differential corpora generated from Go
// (testdata/corpus/NAME: a plain test directory using std's public API)
// on each backend, the Go backend included; outside -short, whose std module
// tests keep samples.
func TestStdCorpora(t *testing.T) {
	if testing.Short() {
		t.Skip("the corpora run outside -short; the std module tests keep samples")
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "corpus", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no corpus directories: %v", err)
	}
	sem := make(chan struct{}, jobs())
	for _, b := range corporaBackends() {
		for _, d := range dirs {
			abs, err := filepath.Abs(d)
			if err != nil {
				t.Fatal(err)
			}
			t.Run(b+"/"+filepath.Base(d), func(t *testing.T) {
				if why, ok := nativeOnlyCorpora[filepath.Base(d)]; ok && b == "go" {
					t.Skip(why)
				}
				if why, ok := goOnlyCorpora[filepath.Base(d)]; ok && b != "go" {
					t.Skip(why)
				}
				t.Parallel()
				sem <- struct{}{}
				defer func() { <-sem }()
				cmd := testtmp.Guard(exec.Command(xo, "test", "--backend="+b, abs))
				cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("xo test --backend=%s %s: %v\n%s", b, d, err, out)
				}
				t.Logf("%s", out)
			})
		}
	}
}

// TestModulesNamed: the walk names a runtime module's test by its
// module path.
func TestModulesNamed(t *testing.T) {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	has := false
	for _, m := range modules(t, root) {
		if m == "native/llrt/scan" {
			has = true
		}
	}
	if !has {
		t.Errorf("std/native/llrt/scan_test.xo is not found as native/llrt/scan")
	}
}
