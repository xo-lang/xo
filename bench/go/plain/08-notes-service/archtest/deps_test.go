package archtest

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const module = "benchtask"

// allowed lists, for each layered package, the project packages it may
// import. Anything else from this module is a violation. External packages
// (the standard library) are not restricted.
var allowed = map[string][]string{
	"domain": {},
	"store":  {"domain"},
	"api":    {"domain", "store"},
}

// projectImports returns the project packages imported by any .go file
// (including tests) in dir, relative to the module path.
func projectImports(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var out []string
	files := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		files++
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if p == module || strings.HasPrefix(p, module+"/") {
				out = append(out, strings.TrimPrefix(p, module+"/"))
			}
		}
	}
	if files == 0 {
		t.Fatalf("no Go files in %s; the rule would pass vacuously", dir)
	}
	return out
}

// violations returns the imports of pkg that break the rules.
func violations(pkg string, imports []string) []string {
	var bad []string
	for _, imp := range imports {
		if imp == pkg {
			continue // external test package importing its own package
		}
		if !slices.Contains(allowed[pkg], imp) {
			bad = append(bad, imp)
		}
	}
	return bad
}

func TestDependencyRules(t *testing.T) {
	for pkg := range allowed {
		imports := projectImports(t, filepath.Join("..", pkg))
		if bad := violations(pkg, imports); len(bad) > 0 {
			t.Errorf("package %s must not import %v", pkg, bad)
		}
	}
}

// TestViolationsDetectsForbiddenImports proves the checker itself fails on
// the forbidden edges the task names.
func TestViolationsDetectsForbiddenImports(t *testing.T) {
	cases := []struct {
		pkg     string
		imports []string
		bad     bool
	}{
		{"domain", []string{"store"}, true},
		{"domain", []string{"api"}, true},
		{"store", []string{"api"}, true},
		{"store", []string{"domain"}, false},
		{"api", []string{"domain", "store"}, false},
		{"api", []string{"api"}, false},
	}
	for _, tc := range cases {
		if got := len(violations(tc.pkg, tc.imports)) > 0; got != tc.bad {
			t.Errorf("violations(%s, %v) bad=%v, want %v", tc.pkg, tc.imports, got, tc.bad)
		}
	}
}
