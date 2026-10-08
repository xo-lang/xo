package check

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
)

var update = flag.Bool("update", false, "rewrite the golden want.json files")

// TestGolden checks each module under testdata/golden and compares the
// diagnostics, as core 10.2 JSON, with want.json. Paths are relative to the
// case directory. Run `go test ./internal/check -run TestGolden -update` to
// rewrite them after an intended change.
func TestGolden(t *testing.T) {
	cases, err := os.ReadDir("testdata/golden")
	if err != nil {
		t.Fatal(err)
	}
	for _, cs := range cases {
		if !cs.IsDir() {
			continue
		}
		t.Run(cs.Name(), func(t *testing.T) {
			dir := filepath.Join("testdata", "golden", cs.Name())
			res, err := Check([]string{dir})
			if err != nil {
				t.Fatal(err)
			}
			ds := res.Diagnostics
			prefix := filepath.ToSlash(dir) + "/"
			for i := range ds {
				relPaths(&ds[i], prefix)
			}
			if ds == nil {
				ds = []diag.Diagnostic{}
			}
			got, err := json.MarshalIndent(ds, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			wantPath := filepath.Join(dir, "want.json")
			if *update {
				if err := os.WriteFile(wantPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if string(got) != string(want) {
				t.Errorf("diagnostics differ from %s (run with -update after checking):\n%s", wantPath, got)
			}
		})
	}
}

func relPaths(d *diag.Diagnostic, prefix string) {
	trim := func(s *diag.Span) {
		s.File = strings.TrimPrefix(filepath.ToSlash(s.File), prefix)
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
