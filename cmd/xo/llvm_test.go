package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestBuildRunLLVM drives `xo build --backend=llvm` and `xo run
// --backend=llvm` (compiler/llvm).
func TestBuildRunLLVM(t *testing.T) {
	t.Parallel()
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(dir, "build"))
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
	if code := run(t, []string{"build", "--backend=llvm", "-o", bin, src}, &out, &errOut); code != 0 {
		t.Fatalf("build: exit %d: %s", code, errOut.String())
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("no binary: %v", err)
	}
	out.Reset()
	errOut.Reset()
	code := run(t, []string{"run", "--backend=llvm", src}, &out, &errOut)
	if code != 70 || out.String() != "42\n" || !strings.Contains(errOut.String(), "fault: integer overflow: 9223372036854775807 * 2 does not fit Int at main.xo:1:28") {
		t.Fatalf("run: exit %d stdout %q stderr %s", code, out.String(), errOut.String())
	}
	// dbg prints its line on stderr and returns the value (decision 0265);
	// a release build only computes the value
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte("fn head(xs: List[Int]) -> Int {\n  dbg(xs.len())\n}\n\nfn main(os: Os) {\n  os.stdio.println(\"${head([3])}\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"run", "--backend=llvm", src}, &out, &errOut); code != 0 || out.String() != "1\n" || errOut.String() != "[main.xo:2:3] xs.len() = 1\n" {
		t.Fatalf("dbg: exit %d stdout %q stderr %q", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"build", "--backend=llvm", "--release", "-o", bin, src}, &out, &errOut); code != 0 {
		t.Fatalf("dbg --release: exit %d stderr %s", code, errOut.String())
	}
	if res, err := command(t, bin).CombinedOutput(); err != nil || string(res) != "1\n" {
		t.Fatalf("dbg --release: %v %q", err, res)
	}
	// constructs outside the subset are build errors naming them
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte("fn main(os: Os) {\n  let m: Map[Set[Int], Int] = {}\n  os.stdio.println(\"${m.len()}\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	errOut.Reset()
	if code := run(t, []string{"build", "--backend=llvm", "-o", bin, src}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "main.xo:2:31: error XO1201: not available on the native backends (arm64, amd64, LLVM) yet: a Map key or Set element that holds a Map or Set (support is planned)") {
		t.Fatalf("unsupported: exit %d stderr %s", code, errOut.String())
	}
}

// TestNoGC drives `--no-gc` (decision 0083): the host's native backend
// builds and runs a program, `xo build --json` and `xo check --no-gc`
// report what it cannot build as XO1201 at the site, and --backend=go
// is rejected.
func TestNoGC(t *testing.T) {
	t.Parallel()
	if xotool.NativeBackend() == xotool.BackendLLVM {
		if _, err := xotool.Clang(); err != nil {
			t.Skip("clang not found")
		}
	} else if _, err := xotool.CC(); err != nil {
		t.Skip("cc not found")
	}
	dir := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(dir, "build"))
	src := filepath.Join(dir, "app")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(prog string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(prog), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("fn main(os: Os) {\n  os.stdio.println(\"${[1, 2, 3].len()}\")\n}\n")
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--no-gc", src}, &out, &errOut); code != 0 || out.String() != "3\n" {
		t.Fatalf("run --no-gc: exit %d stdout %q stderr %s", code, out.String(), errOut.String())
	}
	write("fn main(os: Os) {\n  let m: Map[Set[Int], Int] = {}\n  os.stdio.println(\"${m.len()}\")\n}\n")
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"build", "--no-gc", "--json", src}, &out, &errOut); code != 1 {
		t.Fatalf("build --no-gc: exit %d stderr %s", code, errOut.String())
	}
	var got struct {
		Ok          bool
		Diagnostics []diagnostic
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if got.Ok || len(got.Diagnostics) != 1 {
		t.Fatalf("want one diagnostic: %s", out.String())
	}
	d := got.Diagnostics[0]
	if d.Code != "XO1201" || d.Span.Line != 2 || d.Span.Col != 31 || !strings.Contains(d.Message, "Map key or Set element") || !strings.Contains(d.Message, "planned") {
		t.Errorf("diagnostic: %+v", d)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"check", "--no-gc", src}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "XO1201") {
		t.Errorf("check --no-gc: exit %d stderr %s", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"check", src}, &out, &errOut); code != 0 {
		t.Errorf("check: exit %d stderr %s", code, errOut.String())
	}
	errOut.Reset()
	if code := run(t, []string{"build", "--no-gc", "--backend=go", src}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "native backend") {
		t.Errorf("--backend=go: exit %d stderr %s", code, errOut.String())
	}
}
