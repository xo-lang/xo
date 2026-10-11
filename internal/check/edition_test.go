package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const editionMain = "fn main() {\n}\n"

func TestEditionRecorded(t *testing.T) {
	cases := []struct {
		mod  string
		want string
	}{
		{"module example.com/a\nedition preview\n", "preview"},
		{"module example.com/a\n", CurrentEdition},
		{"edition preview\nmodule example.com/a\n", "preview"},
		// The old name is an alias of preview (decision 0155).
		{"module example.com/a\nedition 2026\n", "preview"},
	}
	for _, tc := range cases {
		dir := writeTree(t, map[string]string{"xo.mod": tc.mod, "main.xo": editionMain})
		res, err := Check([]string{dir})
		if err != nil {
			t.Fatalf("%q: %v", tc.mod, err)
		}
		if len(res.Modules) != 1 || res.Modules[0].Edition != tc.want {
			t.Errorf("%q: edition %q, want %q", tc.mod, res.Modules[0].Edition, tc.want)
		}
	}
	// No xo.mod at all: the current edition.
	dir := writeTree(t, map[string]string{"m/main.xo": editionMain})
	res, err := Check([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Modules[0].Edition != CurrentEdition {
		t.Errorf("no xo.mod: edition %q", res.Modules[0].Edition)
	}
}

func TestEditionUnsupported(t *testing.T) {
	for _, mod := range []string{"module x.com/a\nedition 2031\n", "module x.com/a\nedition 2027\n", "module x.com/a\nedition next\n", "module x.com/a\nedition\n"} {
		dir := writeTree(t, map[string]string{"xo.mod": mod, "main.xo": editionMain})
		_, err := Check([]string{dir})
		if err == nil || !strings.Contains(err.Error(), "edition") {
			t.Errorf("%q: err = %v, want an edition error", mod, err)
		}
	}
}

// TestEditionAliasNote checks the fixable note XO1315 on `edition 2026`
// (decision 0155): once per xo.mod, on the alias, with a fix that writes
// `preview`, and none on `edition preview`.
func TestEditionAliasNote(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"xo.mod":     "module x.com/a\n  edition 2026\n",
		"main.xo":    "use x.com/a/sub\n\nfn main() {\n  sub.f()\n}\n",
		"sub/sub.xo": "pub fn f() {\n}\n",
	})
	res, err := Check([]string{dir, filepath.Join(dir, "sub")})
	if err != nil {
		t.Fatal(err)
	}
	var got []diag.Diagnostic
	for _, d := range res.Diagnostics {
		if d.Code == string(diag.EditionAlias) {
			got = append(got, d)
		}
	}
	if len(got) != 1 {
		t.Fatalf("XO1315 reported %d times, want once: %v", len(got), codes(res.Diagnostics))
	}
	d := got[0]
	if d.Severity != diag.Note || !strings.HasSuffix(d.Span.File, "xo.mod") || d.Span.Line != 2 || d.Span.Col != 11 || d.Span.EndCol != 15 {
		t.Errorf("diagnostic %+v", d)
	}
	if len(d.Fixes) != 1 || len(d.Fixes[0].Edits) != 1 || d.Fixes[0].Edits[0].Replace != "preview" {
		t.Errorf("fixes %+v", d.Fixes)
	}

	dir = writeTree(t, map[string]string{"xo.mod": "module x.com/a\nedition preview\n", "main.xo": editionMain})
	res, err = Check([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == string(diag.EditionAlias) {
			t.Errorf("XO1315 on edition preview: %+v", d)
		}
	}
}

func TestResolveEdition(t *testing.T) {
	for in, want := range map[string]string{"preview": "preview", "2026": "preview", "2027": "", "": "", "Preview": ""} {
		got, ok := ResolveEdition(in)
		if got != want || ok != (want != "") {
			t.Errorf("ResolveEdition(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

// TestEditionRuleHook registers a rule for a future edition and checks that
// it applies to modules on that edition only, which is how a later edition
// changes checker behavior without changing the meaning of older code.
func TestEditionRuleHook(t *testing.T) {
	oldEds, oldRules := Editions, editionRules
	defer func() { Editions, editionRules = oldEds, oldRules }()
	Editions = append(append([]string{}, Editions...), "next")
	editionRules = append(append([]editionRule{}, editionRules...), editionRule{
		Since: "next",
		Name:  "test-rule",
		Check: func(c *checker, f *File) {
			c.warnf(diag.UnusedEffect, token.Span{File: f.Path, Start: token.Pos{Line: 1, Col: 1}}, "edition next rule")
		},
	})
	has := func(mod string) bool {
		dir := writeTree(t, map[string]string{"xo.mod": mod, "main.xo": editionMain})
		res, err := Check([]string{dir})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range res.Diagnostics {
			if d.Message == "edition next rule" {
				return true
			}
		}
		return false
	}
	if has("module x.com/a\nedition preview\n") {
		t.Error("next rule applied to a preview module")
	}
	if has("module x.com/a\nedition 2026\n") {
		t.Error("next rule applied to a module on the preview alias")
	}
	if !has("module x.com/a\nedition next\n") {
		t.Error("next rule not applied to a next module")
	}
}

func TestMapIndexReadFix(t *testing.T) {
	ds := checkSrc(t, "fn f(m: Map[Str, Int]) -> Int {\n  m[\"a\"]\n}\n")
	for _, d := range ds {
		if d.Code != string(diag.MapIndexRead) {
			continue
		}
		if len(d.Fixes) != 1 || len(d.Fixes[0].Edits) != 2 {
			t.Fatalf("fixes = %+v", d.Fixes)
		}
		if d.Fixes[0].Edits[0].Replace != ".get_or_fault(" {
			t.Errorf("first edit %+v", d.Fixes[0].Edits[0])
		}
		return
	}
	t.Fatalf("no XO0410 in %v", codes(ds))
}
