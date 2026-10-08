package archtest

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const module = "benchtask"

// allowed lists, for each package directory, the module packages it may
// import. Anything else under the module path is a violation.
var allowed = map[string][]string{
	"domain": {},
	"store":  {"domain"},
	"api":    {"domain", "store"},
	"server": {"domain", "store", "api"},
}

// violations parses every .go file (including tests) in dir and returns a
// description of each module import not in permitted.
func violations(dir string, permitted []string) ([]string, error) {
	ok := map[string]bool{}
	for _, p := range permitted {
		ok[module+"/"+p] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var out []string
	files := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		files++
		path := filepath.Join(dir, e.Name())
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}
		for _, imp := range f.Imports {
			if imp == nil || imp.Path == nil {
				continue
			}
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return nil, err
			}
			if (p == module || strings.HasPrefix(p, module+"/")) && !ok[p] {
				out = append(out, e.Name()+" imports "+p)
			}
		}
	}
	if files == 0 {
		out = append(out, "no Go files found in "+dir)
	}
	sort.Strings(out)
	return out, nil
}

func TestDependencyRules(t *testing.T) {
	for pkg, permitted := range allowed {
		v, err := violations(filepath.Join("..", pkg), permitted)
		if err != nil {
			t.Fatalf("%s: %v", pkg, err)
		}
		for _, msg := range v {
			t.Errorf("%s: forbidden dependency: %s", pkg, msg)
		}
	}
}

// TestDetectorCatchesViolations proves the rule test would actually fail if
// domain imported store or api, or store imported api.
func TestDetectorCatchesViolations(t *testing.T) {
	cases := []struct {
		pkg, imp string
	}{
		{"domain", "benchtask/store"},
		{"domain", "benchtask/api"},
		{"store", "benchtask/api"},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		src := "package " + tc.pkg + "\n\nimport _ \"" + tc.imp + "\"\n"
		if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		v, err := violations(dir, allowed[tc.pkg])
		if err != nil {
			t.Fatal(err)
		}
		if len(v) != 1 {
			t.Errorf("%s importing %s: got violations %v, want exactly one", tc.pkg, tc.imp, v)
		}
	}
}
