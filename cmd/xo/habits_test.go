package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateHabits = flag.Bool("update-habits", false, "rewrite testdata/habits/*/want.json and fixed.xo from the shipped xo")

// TestHabitGolden checks the habit diagnostics of decision 0087 (and
// compiler/check habits.xo's hint table) against the shipped xo. Each case
// in testdata/habits holds in.xo, a program written with one habit from
// another language; want.json, the diagnostics `xo check --json` reports
// for it (core 10.2), with paths relative to the program's directory;
// and, when the habit has a machine fix, fixed.xo, the program after `xo
// fix`, which must pass `xo check`. A case without fixed.xo is a habit
// with no unambiguous rewrite. `go test ./cmd/xo -run TestHabitGolden
// -update-habits` rewrites the expectations after an intended change:
// review the diff. (The Go test of internal/fix ran these until decision
// 0215 deleted it.)
func TestHabitGolden(t *testing.T) {
	t.Parallel()
	cases, err := os.ReadDir(filepath.Join("testdata", "habits"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, cs := range cases {
		if !cs.IsDir() {
			continue
		}
		n++
		name := cs.Name()
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			caseDir := filepath.Join("testdata", "habits", name)
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
			var out, errOut bytes.Buffer
			run(t, []string{"check", "--json", dir}, &out, &errOut)
			var res struct {
				Diagnostics json.RawMessage `json:"diagnostics"`
			}
			if err := json.Unmarshal(out.Bytes(), &res); err != nil {
				t.Fatalf("xo check --json: %v\n%s%s", err, out.String(), errOut.String())
			}
			var ind bytes.Buffer
			if err := json.Indent(&ind, res.Diagnostics, "", "  "); err != nil {
				t.Fatal(err)
			}
			// paths relative to the program's directory
			got := strings.ReplaceAll(ind.String(), filepath.ToSlash(dir)+"/", "") + "\n"
			if got == "null\n" || got == "[]\n" {
				t.Fatal("no diagnostic: the habit was not reported")
			}
			var ds []struct {
				Fixes []json.RawMessage `json:"fixes"`
			}
			if err := json.Unmarshal(res.Diagnostics, &ds); err != nil {
				t.Fatal(err)
			}
			hasFix := false
			for _, d := range ds {
				hasFix = hasFix || len(d.Fixes) > 0
			}
			out.Reset()
			errOut.Reset()
			run(t, []string{"fix", dir}, &out, &errOut)
			after, err := os.ReadFile(main)
			if err != nil {
				t.Fatal(err)
			}
			wantPath := filepath.Join(caseDir, "want.json")
			fixedPath := filepath.Join(caseDir, "fixed.xo")
			if *updateHabits {
				if err := os.WriteFile(wantPath, []byte(got), 0o644); err != nil {
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
				t.Fatalf("%v (run with -update-habits to create it)", err)
			}
			if got != string(want) {
				t.Errorf("diagnostics differ from %s (rerun with -update-habits after reading):\n%s", wantPath, got)
			}
			wantFixed, err := os.ReadFile(fixedPath)
			switch {
			case err == nil:
				if string(after) != string(wantFixed) {
					t.Errorf("the fixed program differs from %s:\n%s", fixedPath, after)
				}
				out.Reset()
				errOut.Reset()
				if code := run(t, []string{"check", dir}, &out, &errOut); code != 0 {
					t.Errorf("the fixed program does not check: exit %d\n%s", code, errOut.String())
				}
			case os.IsNotExist(err):
				if hasFix {
					t.Errorf("the diagnostic has a fix but %s is missing (run with -update-habits)", fixedPath)
				}
			default:
				t.Fatal(err)
			}
		})
	}
	if n == 0 {
		t.Fatal("no habit cases")
	}
}
