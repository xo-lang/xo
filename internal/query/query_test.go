package query

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, files map[string]string) (*Session, string) {
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
	s, err := Load([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func TestResolveAcrossModules(t *testing.T) {
	s, _ := load(t, map[string]string{
		"xo.mod":      "module example.com/app\n",
		"a/a.xo":      "pub fn run() -> Int { 1 }\n\npub enum Color { Red, Blue(n: Int) }\n",
		"b/b.xo":      "use example.com/app/a\n\npub fn run() -> Int { a.run() + helper() }\n\nfn helper() -> Int { 2 }\n",
		"b/b_test.xo": "test \"t\" {\n  expect(run() == 3)\n}\n",
	})
	if _, err := s.Effects("run"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("unqualified run: %v", err)
	}
	r, err := s.Callers("a.run")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Callers) != 1 || r.Callers[0].Caller != "run" || r.Callers[0].Module != "example.com/app/b" {
		t.Errorf("callers of a.run = %+v", r.Callers)
	}
	r, err = s.Callers("example.com/app/b.run")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Callers) != 1 || !strings.HasPrefix(r.Callers[0].Caller, "test ") {
		t.Errorf("callers of b.run = %+v", r.Callers)
	}
	d, err := s.Def("a.Color.Blue")
	if err != nil || len(d) != 1 || d[0].Kind != "variant" || d[0].Signature != "Blue(n: Int)" {
		t.Errorf("def a.Color.Blue = %+v, %v", d, err)
	}
	e, err := s.Effects("b.helper")
	if err != nil || !e.Inferred || len(e.Performed) != 0 {
		t.Errorf("effects b.helper = %+v, %v", e, err)
	}
	api, err := s.API("example.com/app/b")
	if err != nil || len(api.Items) != 1 {
		t.Errorf("api = %+v, %v", api, err)
	}
	if _, err := s.API("example.com/none"); err == nil {
		t.Error("api of a missing module")
	}
}
