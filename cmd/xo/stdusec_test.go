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
