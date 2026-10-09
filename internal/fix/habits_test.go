package fix

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
)

var update = flag.Bool("update", false, "rewrite testdata/habits/*/want.json and fixed.xo")

// TestHabitGolden checks the habit diagnostics of decision 0087. Each case
// in testdata/habits holds in.xo, a program written with one habit from
// another language; want.json, the diagnostics `xo check` reports for it
// (core 10.2 JSON); and, when the habit has a machine fix, fixed.xo, the
// program after `xo fix`, which must pass `xo check`. A case without
// fixed.xo is a habit with no unambiguous rewrite: its diagnostic must not
// carry a fix. Run `go test ./internal/fix -run TestHabitGolden -update`
// to rewrite the expectations after an intended change, then review them.
func TestHabitGolden(t *testing.T) {
	cases, err := os.ReadDir("testdata/habits")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no habit cases")
	}
	for _, cs := range cases {
		if !cs.IsDir() {
			continue
		}
		t.Run(cs.Name(), func(t *testing.T) {
			caseDir := filepath.Join("testdata", "habits", cs.Name())
			src, err := os.ReadFile(filepath.Join(caseDir, "in.xo"))
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), "m")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			main := filepath.Join(dir, "main.xo")
			if err := os.WriteFile(main, src, 0o644); err != nil {
				t.Fatal(err)
			}
			// The diagnostics.
			res, err := check.Check([]string{dir})
			if err != nil {
				t.Fatal(err)
			}
			ds := res.Diagnostics
			for i := range ds {
				relHabitPaths(&ds[i], dir)
			}
			if ds == nil {
				ds = []diag.Diagnostic{}
			}
			hasFix := false
			for _, d := range ds {
				hasFix = hasFix || len(d.Fixes) > 0
			}
			if len(ds) == 0 {
				t.Fatal("no diagnostic: the habit was not reported")
			}
			got, err := json.MarshalIndent(ds, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			wantPath := filepath.Join(caseDir, "want.json")
			fixedPath := filepath.Join(caseDir, "fixed.xo")
			// The fixes.
			fr, err := Run([]string{dir}, Options{})
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(main)
			if err != nil {
				t.Fatal(err)
			}
			if *update {
				if err := os.WriteFile(wantPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
				os.Remove(fixedPath)
				if hasFix {
					if err := os.WriteFile(fixedPath, after, 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if string(got) != string(want) {
				t.Errorf("diagnostics differ from %s (run with -update after checking):\n%s", wantPath, got)
			}
			wantFixed, err := os.ReadFile(fixedPath)
			switch {
			case err == nil:
				if string(after) != string(wantFixed) {
					t.Errorf("fixed program differs from %s:\n%s", fixedPath, after)
				}
				if len(fr.Remaining) > 0 {
					var msgs []string
					for _, d := range fr.Remaining {
						msgs = append(msgs, diag.Human(d))
					}
					t.Errorf("errors remain after the fixes:\n%s", strings.Join(msgs, "\n"))
				}
				// The fixed program passes `xo check`.
				res2, err := check.Check([]string{dir})
				if err != nil {
					t.Fatal(err)
				}
				for _, d := range res2.Diagnostics {
					if d.Severity == diag.Error {
						t.Errorf("fixed program does not check: %s", diag.Human(d))
					}
				}
			case os.IsNotExist(err):
				if hasFix {
					t.Errorf("the diagnostic has a fix but %s is missing (run with -update)", fixedPath)
				}
			default:
				t.Fatal(err)
			}
		})
	}
}

func relHabitPaths(d *diag.Diagnostic, dir string) {
	trim := func(s *diag.Span) {
		if rel, err := filepath.Rel(dir, s.File); err == nil {
			s.File = filepath.ToSlash(rel)
		}
	}
	trim(&d.Span)
	for i := range d.Notes {
		if d.Notes[i].Span != nil {
			trim(d.Notes[i].Span)
		}
	}
	for i := range d.Fixes {
		for j := range d.Fixes[i].Edits {
			trim(&d.Fixes[i].Edits[j].Span)
		}
	}
}
