// Package stdtest runs the Xo tests of std modules (decision 0262): each
// std/.../NAME_test.xo beside its module's source is run with `xo test
// --std-module=MODULE`, so the tests have the module's private
// functions. Backends by tier: -short the host's native backend (arm64
// or amd64, decision 0270; llvm on another host); the default LLVM and
// the native one; XO_STDTEST_BACKENDS (comma separated) names them
// instead.
package stdtest

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

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
	for _, b := range backends() {
		for _, m := range mods {
			// the runtime's modules (llrt) and the implementations run on
			// the native backends only (decision 0262 item 4)
			t.Run(b+"/"+m, func(t *testing.T) {
				cmd := exec.Command(xo, "test", "--std-module="+m, "--backend="+b)
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
