package arm64

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestBuildDirBelowCwdSameBinary builds one program twice with a build
// directory below the working directory, as bootstrap-xo.sh does with an
// OUT in the checkout: the binaries must be the same. --ld=cc, since Xo's
// linker writes no C source to record (a hello program uses it by default,
// and this test then passed without the fix). The objects of the
// generated C sources (xo_buildinfo.c, xo_tzdata.c) recorded each build's
// own directory, so s2 and s3 differed on Linux, in .debug_line_str, with an
// OUT below the checkout and not with one outside it (compiler/xo/cffi.xo
// c_objects, gotchas/generated-c-path-in-debug-info.md).
func TestBuildDirBelowCwdSameBinary(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds a program")
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module samebin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "fn main(os: Os) {\n  os.stdio.println(\"hello\")\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(stage string) []byte {
		t.Helper()
		c := filepath.Join(dir, ".b", "cache-"+stage)
		bin := filepath.Join(dir, ".b", stage, "prog")
		if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "--ld=cc", "--std="+filepath.Join(root, "std"), "-o", bin, "."))
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "PWD="+dir, "XO_BUILD_DIR="+filepath.Join(c, "build"),
			"XO_RTOBJ_CACHE="+filepath.Join(c, "rtobj"), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(c, "backend"), "XO_BIN_CACHE=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", stage, err, out)
		}
		b, err := os.ReadFile(bin)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	a, b := build("s2"), build("s3")
	if !bytes.Equal(a, b) {
		for _, s := range []string{"cache-s2", "cache-s3"} {
			if bytes.Contains(a, []byte(s)) || bytes.Contains(b, []byte(s)) {
				t.Errorf("a binary records its build directory %s", s)
			}
		}
		t.Errorf("two builds differ: %d and %d bytes", len(a), len(b))
	}
}
