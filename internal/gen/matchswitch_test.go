package gen

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestMatchLiteralSwitch: a match whose arms are all Str or Int literals
// (or-patterns of them) and an optional wildcard is a Go switch on the
// subject, which Go compiles to a length dispatch for strings, not a
// chain of == (task #429). A value an earlier arm took is dropped, so Go
// sees no duplicate case; an arm whose values all were is left out; a
// guarded match keeps the chain. The program's output is the golden
// match_literal_switch.
func TestMatchLiteralSwitch(t *testing.T) {
	t.Parallel()
	bd := t.TempDir()
	dir, err := filepath.Abs(filepath.Join("testdata", "golden", "match_literal_switch"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := xotool.Build([]string{dir}, xotool.Options{Env: []string{"XO_BUILD_DIR=" + bd}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Dir != "" {
		defer os.RemoveAll(r.Dir)
	}
	if r.Failed {
		t.Fatalf("build failed:\n%s", r.Output)
	}
	var src string
	filepath.WalkDir(bd, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "xo.go" && strings.Contains(filepath.ToSlash(p), "match_literal_switch") {
			b, _ := os.ReadFile(p)
			src = string(b)
		}
		return nil
	})
	if src == "" {
		t.Fatalf("no generated xo.go under %s", bd)
	}
	fn := func(name string) string {
		i := strings.Index(src, "func F_"+name+"(")
		if i < 0 {
			i = strings.Index(src, "func "+name+"(")
		}
		if i < 0 {
			t.Fatalf("no func %s in the generated Go:\n%s", name, src)
		}
		j := strings.Index(src[i:], "\n}\n")
		return src[i : i+j]
	}
	for _, c := range []struct {
		fn   string
		want []string
		not  []string
	}{
		{fn: "kind", want: []string{"switch v_w {", "case \"\":", "case \"a\", \"b\":", "case \"c\":", "default:"}, not: []string{"dead b", "== \""}},
		{fn: "small", want: []string{"switch v_n {", "case int64(0):", "case int64(1), int64(-1):", "default:"}, not: []string{"dead one"}},
		{fn: "guarded", want: []string{"switch {"}, not: []string{"switch v_n {"}},
	} {
		body := fn(c.fn)
		for _, w := range c.want {
			if !strings.Contains(body, w) {
				t.Errorf("%s: the generated Go has no %q:\n%s", c.fn, w, body)
			}
		}
		for _, n := range c.not {
			if strings.Contains(body, n) {
				t.Errorf("%s: the generated Go has %q:\n%s", c.fn, n, body)
			}
		}
	}
}
