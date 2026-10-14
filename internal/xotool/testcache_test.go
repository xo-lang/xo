package xotool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestBuildTempCleaned: a Build without Options.Out puts its binary under
// the process's temporary root, and CleanupTemp (TestMain, after m.Run)
// leaves nothing in $TMPDIR; before, every such build left an
// xo-build-* directory there (about 70 GB on 2026-10-14).
func TestBuildTempCleaned(t *testing.T) {
	if _, err := Path(); err != nil {
		t.Skipf("no xo: %v", err)
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	tempMu.Lock()
	saved := tempRoot
	tempRoot = ""
	tempMu.Unlock()
	t.Cleanup(func() {
		tempMu.Lock()
		tempRoot = saved
		tempMu.Unlock()
	})
	res, err := Build([]string{src}, Options{})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %+v", err, res)
	}
	if !strings.HasPrefix(res.Dir, tmp) || !strings.Contains(res.Dir, tempPrefix) {
		t.Errorf("build directory %s is not under this process's root in %s", res.Dir, tmp)
	}
	if !isFile(res.Binaries[Host()]) {
		t.Fatalf("no binary at %s", res.Binaries[Host()])
	}
	CleanupTemp()
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Errorf("CleanupTemp left %d entries in $TMPDIR (first %s)", len(left), left[0].Name())
	}
}
