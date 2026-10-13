package build

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/gotool"
)

// TestGoTooOld builds with a fake go command that reports go1.22.5: the
// Go backend stops with XO1502 before generating code, and the native
// backends do not ask (decision 0176).
func TestGoTooOld(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake go command is a shell script")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "go")
	os.WriteFile(fake, []byte("#!/bin/sh\nif [ \"$1\" = env ] && [ \"$2\" = GOVERSION ]; then echo go1.22.5; exit 0; fi\necho unexpected go \"$@\" >&2; exit 2\n"), 0o755)
	t.Setenv("XO_GO", fake)
	t.Setenv("XO_BUILD_DIR", t.TempDir())
	src := filepath.Join(dir, "hello")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644)
	for _, mode := range []gen.Mode{gen.Build, gen.Test} {
		res, err := Build([]string{src}, Options{Mode: mode})
		if err != nil {
			t.Fatalf("mode %v: %v", mode, err)
		}
		var got []diag.Diagnostic
		for _, d := range res.Diagnostics {
			if d.Code == string(diag.GoTooOld) {
				got = append(got, d)
			}
		}
		if !res.Failed || len(got) != 1 || !strings.Contains(got[0].Message, "go1.22.5") || !strings.Contains(got[0].Message, "Go "+gotool.MinGo+" or newer") {
			t.Errorf("mode %v: failed %v, diagnostics %+v; want one XO1502 naming go1.22.5", mode, res.Failed, res.Diagnostics)
		}
	}
}

// goCommandExempt are go subcommands that read no build cache.
var goCommandExempt = map[string]bool{"env": true, "version": true}

// TestGoCommandsUseXoCache keeps every go command the repo runs on Xo's
// capped Go cache (decision 0176): an exec.Command or CommandContext of
// "go", goTool(), or gotool.Path() whose subcommand may build (anything
// but `go env` and `go version`) must be in a function that sets the
// environment with GoEnv(), goVetEnv(), gotool.CacheEnv(), or a literal
// "GOCACHE=". The default cache grew to 111 GB and then 19 GB on
// 2026-10-11 from go commands that missed it
// (gotchas/go-vet-recompiles-rt-per-build-dir.md). A go command that
// must use the default cache says why in a `//xo:default-gocache`
// comment in the function. The function must also hold the cache's
// shared lock (gotool.HoldCache) while the command runs: a trim in
// another process removes even recent entries once it holds the lock
// exclusively (gotchas/go-commands-outside-build-race-trim.md).
func TestGoCommandsUseXoCache(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	skipDirs := map[string]bool{"runs": true, "output": true, "testdata": true, "third_party": true, ".claude": true, ".tools": true, ".git": true, "node_modules": true}
	checked := 0
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if skipDirs[d.Name()] || rel == filepath.Join("bench", "paper") || rel == filepath.Join("bench", "go") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil || !strings.Contains(string(src), "exec.Command") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, src, parser.ParseComments)
		if err != nil {
			return nil
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			body := string(src[fset.Position(fd.Pos()).Offset:fset.Position(fd.End()).Offset])
			routed := strings.Contains(body, "GoEnv()") || strings.Contains(body, "goVetEnv()") ||
				strings.Contains(body, "CacheEnv()") || strings.Contains(body, `"GOCACHE=`) ||
				commentIn(f, fset, fd, "xo:default-gocache")
			held := strings.Contains(body, "HoldCache()") || commentIn(f, fset, fd, "xo:default-gocache")
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext" {
					return true
				}
				if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "exec" {
					return true
				}
				args := call.Args
				if sel.Sel.Name == "CommandContext" && len(args) > 0 {
					args = args[1:]
				}
				if len(args) == 0 || !isGoCommand(args[0]) {
					return true
				}
				checked++
				if len(args) > 1 {
					if lit, ok := args[1].(*ast.BasicLit); ok {
						if s, err := strconv.Unquote(lit.Value); err == nil && goCommandExempt[s] {
							return true
						}
					}
				}
				if !routed {
					t.Errorf("%s:%d: go command without Xo's Go cache: set cmd.Env from build.GoEnv() (or gotool.CacheEnv()), or explain with //xo:default-gocache", rel, fset.Position(call.Pos()).Line)
				} else if !held {
					t.Errorf("%s:%d: go command on Xo's Go cache without its lock: hold gotool.HoldCache() while it runs, or a trim can remove the entries it reads (gotchas/go-commands-outside-build-race-trim.md)", rel, fset.Position(call.Pos()).Line)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 5 {
		t.Errorf("found only %d go commands; the scan is broken", checked)
	}
}

// isGoCommand reports whether e names the go command: "go", goTool(), or
// gotool.Path().
func isGoCommand(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.BasicLit:
		s, err := strconv.Unquote(e.Value)
		return err == nil && s == "go"
	case *ast.CallExpr:
		switch fn := e.Fun.(type) {
		case *ast.Ident:
			return fn.Name == "goTool"
		case *ast.SelectorExpr:
			return fn.Sel.Name == "Path" && isIdent(fn.X, "gotool")
		}
	}
	return false
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

// commentIn reports whether a comment inside fd contains text.
func commentIn(f *ast.File, fset *token.FileSet, fd *ast.FuncDecl, text string) bool {
	for _, cg := range f.Comments {
		if cg.Pos() >= fd.Pos() && cg.End() <= fd.End() && strings.Contains(cg.Text(), text) {
			return true
		}
	}
	return false
}
