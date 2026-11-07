package main

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdModuleUseC covers a std module with a `use c` line
// (gotchas/std-use-c-binding-types.md): its native file names a handle
// type of the binding in a struct field and a signature, which checks only
// when the binding is declared with std's tree, and its header and link
// paths are under the std directory, so a program in another directory
// builds it. A copy of std gains the module std/ctiny.
func TestStdModuleUseC(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("clang"); err != nil && os.Getenv("XO_CLANG") == "" {
		t.Skip("clang not found")
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	std := filepath.Join(root, "std")
	copyStd := filepath.Join(t.TempDir(), "std")
	err = filepath.WalkDir(std, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(std, p)
		if e.IsDir() {
			if e.Name() == "testdata" || e.Name() == ".xo" || rel == "rt" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(copyStd, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(copyStd, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{
		"ctiny.xo":             "// std/ctiny: a test module (cmd/xo TestStdModuleUseC).\n\nfn sum(a: Int, b: Int) -> Int\n",
		"native/ctiny/ctiny.h": "typedef struct ctiny_h ctiny_h;\nctiny_h *ctiny_new(int v);\nint ctiny_get(ctiny_h *h);\nvoid ctiny_free(ctiny_h *h);\n",
		"native/ctiny/ctiny.c": "#include <stdlib.h>\n#include \"ctiny.h\"\nstruct ctiny_h { int v; };\nctiny_h *ctiny_new(int v) { ctiny_h *h = malloc(sizeof *h); if (h) h->v = v; return h; }\nint ctiny_get(ctiny_h *h) { return h->v; }\nvoid ctiny_free(ctiny_h *h) { free(h); }\n",
		"native/ctiny.xo": `use c "ctiny/ctiny.h" link "ctiny/ctiny.c" as ct uses none

struct Box {
  h: ct.CtinyH
}

fn make(v: Int) -> Box {
  match ct.ctiny_new(Int32(v)) {
    Some(h) => Box{h: h}
    None => fault("ctiny: out of memory")
  }
}

fn get(b: Box) -> Int {
  Int(ct.ctiny_get(b.h))
}

pub fn sum(a: Int, b: Int) -> Int {
  let x = make(a)
  let y = make(b)
  let s = get(x) + get(y)
  ct.ctiny_free(x.h)
  ct.ctiny_free(y.h)
  s
}
`,
	} {
		p := filepath.Join(copyStd, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module ex\n",
		"main.xo": "use std/ctiny\n\nfn main(os: Os) {\n  os.stdio.println(\"${ctiny.sum(2, 40)}\")\n}\n",
	})
	setenv(t, "XO_STD", copyStd)
	setenv(t, "XO_CBIND_DIR", "")
	chdir(t, dir)
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--no-gc", "."}, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != "42" {
		t.Fatalf("xo run: exit %d, stdout %q, want 42\n%s", code, out.String(), errOut.String())
	}
}

// TestStdSQLiteGoBackend: std/sqlite binds C, so the Go backend reports
// XO1201 for a program that uses it (std/go_compiled.txt loads its native
// file there, decision 0534), instead of a Go build that cannot find
// std/rt's Sqlite functions (stdgen generates none for a `use c` module).
func TestStdSQLiteGoBackend(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("checks a program on the Go backend")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module app\n",
		"main.xo": "use std/sqlite\n\nfn main(os: Os) uses fs {\n  let _ = sqlite.version()\n}\n",
	})
	code, out := xoOut(t, "build", "--backend=go", "-o", filepath.Join(t.TempDir(), "p"), dir)
	if code == 0 || !strings.Contains(out, "XO1201") || !strings.Contains(out, "impossible on the Go backend") {
		t.Errorf("want XO1201 (C is impossible on the Go backend), got exit %d:\n%s", code, out)
	}
}

// TestStdSQLiteCrossBuilds builds the native_sqlite golden for each
// cross target hourly.sh checks, every time (its sample of every 4th
// golden reached native_sqlite only after new goldens shifted the list,
// and windows/amd64 had failed since std/sqlite landed on clang's
// "'stdlib.h' file not found", 2026-11-07). A build either links, or
// stops on a documented limit: no cross link with a C compiler
// (linux/amd64 from another host), or no Windows sysroot for a `use c`
// source (XO_WINSYSROOT unset; decision 0534 stage 2's prebuilt is not
// shipped yet). Any other failure, such as a C compiler's error, fails.
func TestStdSQLiteCrossBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("cross builds a program with SQLite's C source")
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "internal", "llvm", "testdata", "golden", "native_sqlite")
	setenv(t, "XO_WINSYSROOT", "")
	limits := []string{"does not cross link", "set XO_WINSYSROOT"}
	for _, tg := range []string{"windows/amd64", "linux/amd64", "darwin/amd64"} {
		code, out := xoOut(t, "build", "--backend=amd64", "--target="+tg, "-o", filepath.Join(t.TempDir(), "p"), dir)
		if code == 0 {
			continue
		}
		limited := false
		for _, l := range limits {
			limited = limited || strings.Contains(out, l)
		}
		if !limited {
			t.Errorf("%s: exit %d, not a documented limit:\n%s", tg, code, out)
		}
	}
}
