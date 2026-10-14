package xotool

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCCCache: CompileC through XO_CC_CACHE stores a binary, reuses it
// for the same inputs, and builds again when a header the source
// includes changes (the key holds clang -MM's headers).
func TestCCCache(t *testing.T) {
	if _, err := Clang(); err != nil {
		t.Skip("clang not found")
	}
	cache := t.TempDir()
	t.Setenv(ccCacheEnv, cache)
	src := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(src, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.c", "#include <stdio.h>\n#include \"val.h\"\nint main(void) { printf(\"%d\\n\", VAL); return 0; }\n")
	runIt := func(want string) {
		t.Helper()
		bin := filepath.Join(t.TempDir(), "prog")
		if err := CompileC([]string{"-O1"}, []string{filepath.Join(src, "main.c")}, bin); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(bin).Output()
		if err != nil || string(out) != want {
			t.Fatalf("got %q, %v; want %q", out, err, want)
		}
	}
	entries := func() int {
		es, _ := filepath.Glob(filepath.Join(cache, "[0-9a-f]*"))
		return len(es)
	}
	write("val.h", "#define VAL 1\n")
	runIt("1\n")
	runIt("1\n")
	if n := entries(); n != 1 {
		t.Fatalf("%d cache entries after two identical compiles, want 1", n)
	}
	write("val.h", "#define VAL 2\n")
	runIt("2\n")
	if n := entries(); n != 2 {
		t.Fatalf("%d cache entries after the header changed, want 2", n)
	}
}
