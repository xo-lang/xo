package diag

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	xotoken "github.com/xo-lang/xo/internal/token"
)

// TestCodesDocumented checks that every Code constant in codes.go has a
// description, a unique value, and the XOnnnn form.
func TestCodesDocumented(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "codes.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	form := regexp.MustCompile(`^XO\d{4}$`)
	seen := map[string]string{}
	n := 0
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				lit := vs.Values[i].(*ast.BasicLit)
				code := strings.Trim(lit.Value, `"`)
				n++
				if !form.MatchString(code) {
					t.Errorf("%s = %q is not of the form XOnnnn", name.Name, code)
				}
				if prev, dup := seen[code]; dup {
					t.Errorf("%s and %s share code %s", prev, name.Name, code)
				}
				seen[code] = name.Name
				if Descriptions[Code(code)] == "" {
					t.Errorf("%s (%s) has no entry in Descriptions", name.Name, code)
				}
			}
		}
	}
	if n != len(Descriptions) {
		t.Errorf("%d constants but %d descriptions", n, len(Descriptions))
	}
}

func TestJSONShape(t *testing.T) {
	var l List
	sp := xotoken.Span{File: "a.xo", Start: xotoken.Pos{Line: 18, Col: 3}, End: xotoken.Pos{Line: 18, Col: 24}}
	l.Errorf(UnexpectedToken, sp, "expected %s", "x")
	b, err := json.Marshal(l.Items[0])
	if err != nil {
		t.Fatal(err)
	}
	want := `{"code":"XO0160","severity":"error","message":"expected x","span":{"file":"a.xo","line":18,"col":3,"end_line":18,"end_col":24},"notes":[],"fixes":[]}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
	if !l.HasErrors() {
		t.Error("HasErrors = false")
	}
	if got := Human(*l.Items[0]); got != "a.xo:18:3: error XO0160: expected x" {
		t.Errorf("Human = %q", got)
	}
}

// TestHumanNoPosition checks that a build-level diagnostic (no file)
// and one of a file without a position print no ":0:0:".
func TestHumanNoPosition(t *testing.T) {
	d := Diagnostic{Code: "XO1503", Severity: Error, Message: "no go"}
	if got := Human(d); got != "error XO1503: no go" {
		t.Errorf("no file: %q", got)
	}
	d.Span.File = "xo.mod"
	if got := Human(d); got != "xo.mod: error XO1503: no go" {
		t.Errorf("file only: %q", got)
	}
}
