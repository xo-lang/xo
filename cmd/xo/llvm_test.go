package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
)

// TestBuildRunLLVM drives `xo build --backend=llvm` and `xo run
// --backend=llvm` (internal/llvm).
func TestBuildRunLLVM(t *testing.T) {
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := t.TempDir()
	t.Setenv("XO_BUILD_DIR", filepath.Join(dir, "build"))
	src := filepath.Join(dir, "app")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	prog := "fn double(n: Int) -> Int { n * 2 }\n\nfn main(os: Os) {\n  os.stdio.println(\"${double(21)}\")\n  os.stdio.println(\"${double(9223372036854775807)}\")\n}\n"
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	bin := filepath.Join(dir, "app.bin")
	if code := run([]string{"build", "--backend=llvm", "-o", bin, src}, &out, &errOut); code != 0 {
		t.Fatalf("build: exit %d: %s", code, errOut.String())
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("no binary: %v", err)
	}
	out.Reset()
	errOut.Reset()
	code := run([]string{"run", "--backend=llvm", src}, &out, &errOut)
	if code != 70 || out.String() != "42\n" || !strings.Contains(errOut.String(), "fault: integer overflow: 9223372036854775807 * 2 does not fit Int at main.xo:1:28") {
		t.Fatalf("run: exit %d stdout %q stderr %s", code, out.String(), errOut.String())
	}
	// constructs outside the subset are build errors naming them
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte("fn head(xs: List[Int]) -> Int {\n  match xs {\n    [first, ..] => first\n    [] => 0\n  }\n}\n\nfn main(os: Os) {\n  os.stdio.println(\"${head([3])}\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	errOut.Reset()
	if code := run([]string{"build", "--backend=llvm", "-o", bin, src}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "main.xo:3:5: the SSA builder does not support list patterns") {
		t.Fatalf("unsupported: exit %d stderr %s", code, errOut.String())
	}
	// the deterministic scheduler is the Go backend's (decision 0068)
	errOut.Reset()
	if code := run([]string{"run", "--backend=llvm", "--seed=3", src}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "--seed needs the Go backend") {
		t.Fatalf("seed: exit %d stderr %s", code, errOut.String())
	}
}
