package serve

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCacheNoticesNewWorkspace checks that a cached program is rechecked
// when an xo.work appears above its module, or changes, even when the
// workspace did not use the module before (decision 0086).
func TestCacheNoticesNewWorkspace(t *testing.T) {
	t.Setenv("XO_WORK", "")
	root := t.TempDir()
	app := filepath.Join(root, "app")
	write := func(name, src string) {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("app/xo.mod", "module example.com/app\nedition 2026\n")
	write("app/a.xo", "pub fn one() -> Int { 1 }\n")
	c := NewCache()
	load := func() bool {
		t.Helper()
		_, hit, err := c.Load([]string{app})
		if err != nil {
			t.Fatal(err)
		}
		return hit
	}
	if load() {
		t.Fatal("first load hit")
	}
	if !load() {
		t.Fatal("second load missed")
	}
	// a new workspace above that does not use the module yet
	write("other/xo.mod", "module example.com/other\nedition 2026\n")
	write("xo.work", "xo 2026\nuse ./other\n")
	if load() {
		t.Error("a new xo.work above the module was not noticed")
	}
	if !load() {
		t.Error("unchanged workspace missed")
	}
	// the workspace now uses the module (same size would still change
	// the modification time; make it differ in size too)
	write("xo.work", "xo 2026\nuse ./other\nuse ./app\n")
	if load() {
		t.Error("a changed xo.work was not noticed")
	}
	os.Remove(filepath.Join(root, "xo.work"))
	if load() {
		t.Error("a removed xo.work was not noticed")
	}
}
