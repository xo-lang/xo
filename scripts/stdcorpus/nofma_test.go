package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"runtime"
	"strings"
	"testing"
)

// The rule of gotchas/go-oracle-fma-arm64.md (agent-rules.md, #147): every
// Go answer in a corpus is computed with fused multiply-add off. These
// tests keep its three parts: the detector that refuses a fused build,
// the environment that carries the flag to every oracle's `go run`, and
// main running the wrapper before anything else.

// TestFusedDetector: the test binary is built as GOFLAGS says; on arm64
// without the flag Go fuses x*y+z, and fused() must see it.
func TestFusedDetector(t *testing.T) {
	off := strings.Contains(os.Getenv("GOFLAGS"), "fmahash=n")
	want := runtime.GOARCH == "arm64" && !off
	if runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64" {
		t.Skipf("fusion on %s not known", runtime.GOARCH)
	}
	if got := fused(); got != want {
		t.Fatalf("fused() = %v on %s (GOFLAGS %q), want %v", got, runtime.GOARCH, os.Getenv("GOFLAGS"), want)
	}
}

func TestNoFMAEnv(t *testing.T) {
	env := noFMAEnv([]string{"HOME=/h", "GOFLAGS=-mod=mod", "PATH=/p"})
	j := strings.Join(env, "\n")
	for _, w := range []string{"XO_STDCORPUS_NOFMA=1", "GOFLAGS=-mod=mod -gcflags=all=-d=fmahash=n", "HOME=/h", "PATH=/p"} {
		if !strings.Contains(j, w) {
			t.Fatalf("noFMAEnv: no %q in %q", w, env)
		}
	}
	if strings.Count(j, "GOFLAGS=") != 1 {
		t.Fatalf("noFMAEnv: GOFLAGS twice: %q", env)
	}
	if e := noFMAEnv([]string{"HOME=/h"}); !strings.Contains(strings.Join(e, "\n"), "GOFLAGS=-gcflags=all=-d=fmahash=n") {
		t.Fatalf("noFMAEnv with no GOFLAGS: %q", e)
	}
}

// TestMainRunsNoFMAFirst: main's first statement is the noFMA wrapper,
// and no generator replaces the environment of the oracles it starts
// (cmd.Env without os.Environ would drop GOFLAGS).
func TestMainRunsNoFMAFirst(t *testing.T) {
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, d := range f.Decls {
		fn, isFn := d.(*ast.FuncDecl)
		if !isFn || fn.Name.Name != "main" || len(fn.Body.List) == 0 {
			continue
		}
		if s, isIf := fn.Body.List[0].(*ast.IfStmt); isIf {
			if a, isA := s.Init.(*ast.AssignStmt); isA && len(a.Rhs) == 1 {
				if c, isC := a.Rhs[0].(*ast.CallExpr); isC {
					if id, isId := c.Fun.(*ast.Ident); isId && id.Name == "noFMA" {
						ok = true
					}
				}
			}
		}
	}
	if !ok {
		t.Fatal("main does not start with `if rc := noFMA(); ...`")
	}
	files, _ := os.ReadDir(".")
	for _, e := range files {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") || e.Name() == "main.go" {
			continue
		}
		b, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		// the environment of a command that runs go (an oracle module);
		// a run of this binary itself (httpproxy's child) is built already
		last := ""
		for i, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, "exec.Command(") {
				last = l
			}
			if strings.Contains(l, ".Env = ") && !strings.Contains(l, "os.Environ()") && strings.Contains(last, `exec.Command("go"`) {
				t.Errorf("%s:%d: an oracle's environment without os.Environ() drops GOFLAGS's %s: %s", e.Name(), i+1, noFMAFlag, strings.TrimSpace(l))
			}
		}
	}
}
