package xotool

import (
	"os"
	"path/filepath"
	"testing"
)

// TestClangKey: the clang cache's key follows the inputs' contents and
// the arguments, not the output's or the LTO object's paths, and the
// calls it must not cache (C sources, no output) are left alone.
func TestClangKey(t *testing.T) {
	clang, err := Clang()
	if err != nil {
		t.Skip("no clang")
	}
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	os.MkdirAll(filepath.Join(dir, "a"), 0o755)
	os.MkdirAll(filepath.Join(dir, "b"), 0o755)
	a := write("a/prog.ll", "; one")
	b := write("b/prog.ll", "; one")
	c := write("b/other.ll", "; two")
	key := func(args ...string) string {
		k, _, _, ok := clangKey(clang, args)
		if !ok {
			return ""
		}
		return k
	}
	k1 := key("-O1", "-Wl,-object_path_lto,"+dir+"/a/lto.o", "-o", dir+"/a/out", a)
	k2 := key("-O1", "-Wl,-object_path_lto,"+dir+"/b/lto.o", "-o", dir+"/b/out", b)
	if k1 == "" || k1 != k2 {
		t.Errorf("same IR and flags, other paths: keys %q and %q", k1, k2)
	}
	if k3 := key("-O1", "-o", dir+"/b/out", c); k3 == k2 || k3 == "" {
		t.Errorf("other IR: key %q", k3)
	}
	if k4 := key("-O2", "-Wl,-object_path_lto,"+dir+"/b/lto.o", "-o", dir+"/b/out", b); k4 == k2 {
		t.Errorf("other flags: same key")
	}
	src := write("b/x.c", "int main(void){return 0;}")
	if key("-o", dir+"/b/x", src) != "" {
		t.Errorf("a C source was cached")
	}
	if key("-c", b) != "" {
		t.Errorf("a call with no -o was cached")
	}
}
