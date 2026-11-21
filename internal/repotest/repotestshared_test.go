package repotest

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// shared.txt lists this package's shared helper files: those declaring a
// name another file uses. pushq.sh's fast checks run every test when a
// push changes one, and only the tests a changed file defines otherwise
// (task #589, pq_fast_skip). A file whose helpers belong to one test says
// so with a `//pushq:owner TestX` line, and then only files defining TestX
// may use them.
const repotestShared = "internal/repotest/shared.txt"

var repotestOwner = regexp.MustCompile(`(?m)^//pushq:owner (Test[A-Za-z0-9_]*)$`)

// TestRepotestShared: shared.txt names exactly the files whose
// declarations another file uses (or that declare TestMain), so pushq
// skips no test a changed helper reaches. Reads files only.
func TestRepotestShared(t *testing.T) {
	fset := token.NewFileSet()
	paths, _ := filepath.Glob("*_test.go")
	decl := map[string]string{}           // a package-level name: its file
	tests := map[string]map[string]bool{} // file: the tests it defines
	owner := map[string]string{}
	mainFile := map[string]bool{}
	files := map[string]*ast.File{}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, p, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[p] = f
		tests[p] = map[string]bool{}
		if m := repotestOwner.FindSubmatch(src); m != nil {
			owner[p] = string(m[1])
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					continue
				}
				n := d.Name.Name
				if n == "TestMain" {
					mainFile[p] = true
					continue
				}
				if strings.HasPrefix(n, "Test") {
					tests[p][n] = true
					continue
				}
				decl[n] = p
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.ValueSpec:
						for _, id := range s.Names {
							decl[id.Name] = p
						}
					case *ast.TypeSpec:
						decl[s.Name.Name] = p
					}
				}
			}
		}
	}
	want := map[string]string{} // file: why it is shared
	for p := range mainFile {
		want[p] = "declares TestMain"
	}
	for p, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if s, ok := n.(*ast.SelectorExpr); ok {
				ast.Inspect(s.X, func(n ast.Node) bool { repotestUse(n, p, decl, owner, tests, want); return true })
				return false
			}
			repotestUse(n, p, decl, owner, tests, want)
			return true
		})
	}
	for p, o := range owner {
		found := false
		for _, ts := range tests {
			found = found || ts[o]
		}
		if !found {
			t.Errorf("%s: //pushq:owner %s, but no file defines %s", p, o, o)
		}
	}
	listed := map[string]bool{}
	f, err := os.Open(filepath.Join(root, repotestShared))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l != "" && !strings.HasPrefix(l, "#") {
			listed[l] = true
		}
	}
	var names []string
	for p := range want {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		if !listed[p] {
			t.Errorf("%s is a shared helper file (%s): add it to %s, or move what other files use into a listed file", p, want[p], repotestShared)
		}
	}
	for p := range listed {
		if want[p] == "" {
			t.Errorf("%s: in %s but no other file uses its declarations (or no such file): drop it", p, repotestShared)
		}
	}
}

// repotestUse records in want a package-level name used outside its file
// (an identifier the parser left unresolved: not a local), unless the file
// using it defines the declaring file's //pushq:owner test.
func repotestUse(n ast.Node, p string, decl, owner map[string]string, tests map[string]map[string]bool, want map[string]string) {
	id, ok := n.(*ast.Ident)
	if !ok || id.Obj != nil {
		return
	}
	df, ok := decl[id.Name]
	if !ok || df == p || want[df] != "" {
		return
	}
	if o := owner[df]; o != "" && tests[p][o] {
		return
	}
	want[df] = id.Name + " used in " + p
}
