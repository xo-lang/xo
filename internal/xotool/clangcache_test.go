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
	// a C source with a local include reads a file the key does not see
	src := write("b/x.c", "#include \"x.h\"\nint main(void){return 0;}")
	if key("-o", dir+"/b/x", src) != "" {
		t.Errorf("a C source with a quoted include was cached")
	}
	// xo's generated C (system headers only) is keyed by its contents
	os.MkdirAll(filepath.Join(dir, "c"), 0o755)
	g1 := write("a/xo_tzdata.c", "#include <stdint.h>\nconst uint8_t z[1] = {1};\n")
	g2 := write("c/xo_tzdata.c", "#include <stdint.h>\nconst uint8_t z[1] = {1};\n")
	g3 := write("b/xo_tzdata.c", "#include <stdint.h>\nconst uint8_t z[1] = {2};\n")
	c1 := key("-O1", "-Wl,-oso_prefix,"+dir+"/a/", "-o", dir+"/a/p", a, g1)
	c2 := key("-O1", "-Wl,-oso_prefix,"+dir+"/c/", "-o", dir+"/c/p", b, g2)
	if c1 == "" || c1 != c2 {
		t.Errorf("same IR and C, another build directory (-oso_prefix): keys %q and %q", c1, c2)
	}
	if c3 := key("-O1", "-Wl,-oso_prefix,"+dir+"/b/", "-o", dir+"/b/p", b, g3); c3 == c2 || c3 == "" {
		t.Errorf("other C contents: key %q", c3)
	}
	if key("-c", b) != "" {
		t.Errorf("a call with no -o was cached")
	}
}
