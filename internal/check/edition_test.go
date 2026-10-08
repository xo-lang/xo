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
		want int
	}{
		{"module example.com/a\nedition 2026\n", 2026},
		{"module example.com/a\n", CurrentEdition},
		{"edition 2026\nmodule example.com/a\n", 2026},
	}
	for _, tc := range cases {
		dir := writeTree(t, map[string]string{"xo.mod": tc.mod, "main.xo": editionMain})
		res, err := Check([]string{dir})
		if err != nil {
			t.Fatalf("%q: %v", tc.mod, err)
		}
		if len(res.Modules) != 1 || res.Modules[0].Edition != tc.want {
			t.Errorf("%q: edition %d, want %d", tc.mod, res.Modules[0].Edition, tc.want)
		}
	}
	// No xo.mod at all: the current edition.
	dir := writeTree(t, map[string]string{"m/main.xo": editionMain})
	res, err := Check([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Modules[0].Edition != CurrentEdition {
		t.Errorf("no xo.mod: edition %d", res.Modules[0].Edition)
	}
}

func TestEditionUnsupported(t *testing.T) {
	for _, mod := range []string{"module x.com/a\nedition 2031\n", "module x.com/a\nedition next\n", "module x.com/a\nedition\n"} {
		dir := writeTree(t, map[string]string{"xo.mod": mod, "main.xo": editionMain})
		_, err := Check([]string{dir})
		if err == nil || !strings.Contains(err.Error(), "edition") {
			t.Errorf("%q: err = %v, want an edition error", mod, err)
		}
	}
}

// TestEditionRuleHook registers a rule for a future edition and checks that
// it applies to modules on that edition only, which is how a later edition
// changes checker behavior without changing the meaning of older code.
func TestEditionRuleHook(t *testing.T) {
	oldEds, oldRules := Editions, editionRules
	defer func() { Editions, editionRules = oldEds, oldRules }()
	Editions = append(append([]int{}, Editions...), 2027)
	editionRules = append(append([]editionRule{}, editionRules...), editionRule{
		Since: 2027,
		Name:  "test-rule",
		Check: func(c *checker, f *File) {
			c.warnf(diag.UnusedEffect, token.Span{File: f.Path, Start: token.Pos{Line: 1, Col: 1}}, "edition 2027 rule")
		},
	})
	has := func(mod string) bool {
		dir := writeTree(t, map[string]string{"xo.mod": mod, "main.xo": editionMain})
		res, err := Check([]string{dir})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range res.Diagnostics {
			if d.Message == "edition 2027 rule" {
				return true
			}
		}
		return false
	}
	if has("module x.com/a\nedition 2026\n") {
		t.Error("2027 rule applied to a 2026 module")
	}
	if !has("module x.com/a\nedition 2027\n") {
		t.Error("2027 rule not applied to a 2027 module")
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
