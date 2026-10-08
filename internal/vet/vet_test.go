package vet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
)

func vetSrc(t *testing.T, files map[string]string) *Report {
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
	r, err := Vet([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func codes(r *Report) map[string][]diag.Diagnostic {
	out := map[string][]diag.Diagnostic{}
	for _, d := range r.Diagnostics {
		out[d.Code] = append(out[d.Code], d)
	}
	return out
}

const prog = `fn parse(s: Str) -> Int ! Error {
  Int.parse(s) ?? 0
}

pub fn read_all(f: Fs, p: Str) -> Str ! Error uses fs.read, clock {
  f.read_text(p)?
}

pub fn quiet() uses net {
}

pub fn tags(xs: List[Str]?) -> List[Str] {
  xs ?? [].map(fn(s: Str) { s.trim() })
}

pub fn fine(xs: List[Str]?) -> Int {
  (xs ?? []).len()
}

pub fn best_effort(f: Fs) uses fs.write {
  let _ = f.remove("tmp").ignore_err("best effort temp file cleanup")
}

pub fn show(x: Int) -> Int {
  dbg(x + 1)
}
`

func TestVetFindings(t *testing.T) {
	r := vetSrc(t, map[string]string{"main.xo": prog})
	if r.CheckErrors != 0 {
		for _, d := range r.Diagnostics {
			t.Log(diag.Human(d))
		}
		t.Fatalf("program has %d check errors", r.CheckErrors)
	}
	c := codes(r)
	for _, d := range r.Diagnostics {
		if d.Severity == diag.Error {
			t.Errorf("vet reported an error: %s", diag.Human(d))
		}
	}
	// XO0703: clock in read_all, net in quiet; fs.read is performed.
	var unused []string
	for _, d := range c[string(diag.UnusedEffect)] {
		unused = append(unused, d.Message)
	}
	got := strings.Join(unused, "\n")
	if len(unused) != 2 || !strings.Contains(got, "`uses clock`") || !strings.Contains(got, "`uses net`") || strings.Contains(got, "fs.read") || strings.Contains(got, "fs.write") {
		t.Errorf("XO0703 findings:\n%s", got)
	}
	if n := len(c[string(diag.CoalesceChainTrap)]); n != 1 {
		t.Errorf("XO0207: %d findings, want 1 (tags, not fine nor parse)", n)
	} else if d := c[string(diag.CoalesceChainTrap)][0]; !strings.Contains(d.Message, "`.map`") || len(d.Fixes) != 1 {
		t.Errorf("XO0207: %s", diag.Human(d))
	}
	if ig := c[string(diag.IgnoredErrorReview)]; len(ig) != 1 || !strings.Contains(ig[0].Message, "best effort temp file cleanup") || ig[0].Severity != diag.Note {
		t.Errorf("XO0208: %v", ig)
	}
	if db := c[string(diag.DebugLeft)]; len(db) != 1 || len(db[0].Fixes) != 1 {
		t.Errorf("XO0901: %v", db)
	}
	if len(c[string(diag.GoUsageReport)]) != 0 {
		t.Errorf("XO1103 without use go")
	}
}

func TestVetKeepsCheckWarnings(t *testing.T) {
	r := vetSrc(t, map[string]string{"main.xo": "pub fn f() -> Int {\n  ???\n}\n"})
	if c := codes(r); len(c[string(diag.Hole)]) != 1 {
		t.Errorf("hole warning not kept: %v", r.Diagnostics)
	}
}

func TestVetSkipsCheckErrors(t *testing.T) {
	r := vetSrc(t, map[string]string{"main.xo": "pub fn f() -> Int {\n  \"x\"\n}\n"})
	if r.CheckErrors == 0 {
		t.Fatal("no check error counted")
	}
	for _, d := range r.Diagnostics {
		if d.Severity == diag.Error {
			t.Errorf("error in vet output: %s", diag.Human(d))
		}
	}
}

func TestVetGoReport(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a Go package")
	}
	t.Setenv("XO_GOBIND_DIR", t.TempDir())
	r := vetSrc(t, map[string]string{"main.xo": `use go "strings"

fn upper(s: Str) -> Str {
  strings.to_upper(s)
}

pub fn shout(s: Str) -> Str uses go {
  upper(s) + "!"
}

pub fn calm(s: Str) -> Str {
  s
}
`})
	if r.CheckErrors != 0 {
		t.Skipf("Go binding unavailable here (%d check errors)", r.CheckErrors)
	}
	g := codes(r)[string(diag.GoUsageReport)]
	if len(g) != 1 {
		t.Fatalf("XO1103: %v", r.Diagnostics)
	}
	if !strings.Contains(g[0].Message, "[strings]") || !strings.Contains(g[0].Message, "upper") || !strings.Contains(g[0].Message, "shout") || strings.Contains(g[0].Message, "calm") {
		t.Errorf("XO1103: %s", diag.Human(g[0]))
	}
}
