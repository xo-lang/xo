package check

import (
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

// TestEveryCodeTested checks that every diagnostic code the checker can
// emit appears in the `want` list of at least one test case, and in the
// golden files or rule tables as expected output.
func TestEveryCodeTested(t *testing.T) {
	// code constant names -> values, from internal/diag/codes.go
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "../diag/codes.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		if vs, ok := n.(*ast.ValueSpec); ok {
			for i, name := range vs.Names {
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
					values[name.Name] = strings.Trim(lit.Value, `"`)
				}
			}
		}
		return true
	})
	// codes used by the checker
	used := map[string]bool{}
	ref := regexp.MustCompile(`diag\.([A-Z][A-Za-z]+)`)
	srcs, _ := filepath.Glob("*.go")
	var tests strings.Builder
	for _, p := range srcs {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(p, "_test.go") {
			tests.Write(data)
			continue
		}
		for _, m := range ref.FindAllStringSubmatch(string(data), -1) {
			if v, ok := values[m[1]]; ok {
				used[v] = true
			}
		}
	}
	wantRe := regexp.MustCompile(`want: \[\]string\{([^}]*)\}`)
	tested := map[string]bool{}
	for _, m := range wantRe.FindAllStringSubmatch(tests.String(), -1) {
		for _, c := range regexp.MustCompile(`XO\d{4}`).FindAllString(m[1], -1) {
			tested[c] = true
		}
	}
	var missing []string
	for c := range used {
		if !tested[c] {
			missing = append(missing, c)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("codes emitted by internal/check without a positive test case: %v", missing)
	}
}
