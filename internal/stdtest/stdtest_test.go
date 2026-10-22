// Package stdtest runs the Xo tests of std modules (decision 0262): each
// std/.../NAME_test.xo beside its module's source is run with `xo test
// --std-module=MODULE`, so the tests have the module's private
// functions. Backends by tier: -short the host's native backend (arm64
// or amd64, decision 0270; llvm on another host); the default LLVM and
// the native one; XO_STDTEST_BACKENDS (comma separated) names them
// instead. Modules run in parallel, XO_STDTEST_JOBS at a time (default
// min(GOMAXPROCS, 4)). The large differential corpora generated from Go
// are plain test directories under testdata/corpus, which TestStdCorpora
// runs outside -short.
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

// jobs is how many xo test processes run at once.
func jobs() int {
	if v, err := strconv.Atoi(os.Getenv("XO_STDTEST_JOBS")); err == nil && v > 0 {
		return v
	}
	return min(runtime.GOMAXPROCS(0), 4)
}

// TestStdModules runs every std module's tests on each backend.
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
				cmd := testtmp.Guard(exec.Command(xo, "test", "--std-module="+m, "--backend="+b))
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
// on each backend; outside -short, whose std module tests keep samples.
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
	for _, b := range backends() {
		for _, d := range dirs {
			abs, err := filepath.Abs(d)
			if err != nil {
				t.Fatal(err)
			}
			t.Run(b+"/"+filepath.Base(d), func(t *testing.T) {
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
