package diag

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// codeConstants parses codes.go and returns every Code constant name and
// value, in declaration order.
func codeConstants(t *testing.T) (names []string, codes []Code) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "codes.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				lit := vs.Values[i].(*ast.BasicLit)
				names = append(names, name.Name)
				codes = append(codes, Code(strings.Trim(lit.Value, `"`)))
			}
		}
	}
	return names, codes
}

// TestExplanationsComplete checks that every Code constant in codes.go has
// a full explanation, that there are no extra entries, and that Explain
// accepts every spelling of each code.
func TestExplanationsComplete(t *testing.T) {
	names, codes := codeConstants(t)
	if len(codes) == 0 {
		t.Fatal("no Code constants found in codes.go")
	}
	known := map[Code]bool{}
	for i, c := range codes {
		known[c] = true
		e, ok := Explanations[c]
		if !ok {
			t.Errorf("%s (%s) has no entry in Explanations", names[i], c)
			continue
		}
		for field, v := range map[string]string{"Title": e.Title, "Detail": e.Detail, "Example": e.Example, "Fix": e.Fix} {
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s (%s): empty %s", names[i], c, field)
			}
		}
		num := strings.TrimPrefix(string(c), "XO")
		for _, form := range []string{string(c), strings.ToLower(string(c)), num, " " + string(c) + " "} {
			out, ok := Explain(form)
			if !ok {
				t.Errorf("Explain(%q) not found", form)
				continue
			}
			if !strings.HasPrefix(out, string(c)+": "+e.Title+"\n") {
				t.Errorf("Explain(%q) does not start with the code and title:\n%s", form, out)
			}
			for _, want := range []string{"\nseverity: ", "\nExample:\n    ", "\nFix:\n    ", e.Detail} {
				if !strings.Contains(out, want) {
					t.Errorf("Explain(%q) is missing %q:\n%s", form, want, out)
				}
			}
		}
	}
	for c := range Explanations {
		if !known[c] {
			t.Errorf("Explanations has %s, which is not a Code constant in codes.go", c)
		}
	}
	if len(Explanations) != len(codes) {
		t.Errorf("%d constants but %d explanations", len(codes), len(Explanations))
	}
}

func TestExplainUnknown(t *testing.T) {
	for _, s := range []string{"XO9999", "xo9999", "9999", "", "XO", "bogus"} {
		if out, ok := Explain(s); ok || out != "" {
			t.Errorf("Explain(%q) = %q, %v; want not found", s, out, ok)
		}
	}
}

func TestExplainFormat(t *testing.T) {
	out, ok := Explain("XO0201")
	if !ok {
		t.Fatal("XO0201 not found")
	}
	if !strings.Contains(out, "\nseverity: error\n") {
		t.Errorf("XO0201 severity is not error:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasSuffix(line, " ") {
			t.Errorf("trailing space in line %q", line)
		}
	}
	for code, want := range map[string]Severity{"XO0305": Warning, "XO0900": Warning, "XO0208": Note, "XO1103": Note, "XO0901": Warning} {
		out, _ := Explain(code)
		if !strings.Contains(out, "\nseverity: "+string(want)+"\n") {
			t.Errorf("%s severity is not %s:\n%s", code, want, out)
		}
	}
}

// TestExplanationsNoEmDash keeps the explanation text free of em-dashes
// (project writing style).
func TestExplanationsNoEmDash(t *testing.T) {
	for c, e := range Explanations {
		for field, v := range map[string]string{"Title": e.Title, "Detail": e.Detail, "Example": e.Example, "Fix": e.Fix} {
			if strings.ContainsRune(v, '\u2014') {
				t.Errorf("%s %s contains an em-dash", c, field)
			}
		}
	}
}
