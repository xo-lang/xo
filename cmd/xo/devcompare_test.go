package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDevCompare: xo dev compare (decision 0264) on two ANN=1 run
// directories prints what bench/perf/http/cgcompare.py printed for them
// (want.txt), and the script still agrees when python3 is on PATH; --json
// gives one object a scenario.
func TestDevCompare(t *testing.T) {
	dir := filepath.Join("testdata", "devcompare")
	base, _ := filepath.Abs(filepath.Join(dir, "base"))
	nw, _ := filepath.Abs(filepath.Join(dir, "new"))
	var out, errOut bytes.Buffer
	if code := run(t, []string{"dev", "compare", base, nw}, &out, &errOut); code != 0 {
		t.Fatalf("xo dev compare: exit %d: %s", code, errOut.String())
	}
	want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != string(want) {
		t.Errorf("xo dev compare:\n%s\nwant (cgcompare.py's):\n%s", out.String(), want)
	}
	if py, err := exec.LookPath("python3"); err == nil {
		script := filepath.Join(repoRoot, "bench", "perf", "http", "cgcompare.py")
		if _, err := os.Stat(script); err == nil {
			got, err := exec.Command(py, script, base, nw).Output()
			if err != nil {
				t.Fatalf("cgcompare.py: %v", err)
			}
			if string(got) != out.String() {
				t.Errorf("cgcompare.py now prints:\n%s\nxo dev compare:\n%s", got, out.String())
			}
		}
	}

	out.Reset()
	if code := run(t, []string{"dev", "compare", "--json", "--top=2", base, nw, "h2"}, &out, &errOut); code != 0 {
		t.Fatalf("--json: exit %d: %s", code, errOut.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("--json h2: %d lines:\n%s", len(lines), out.String())
	}
	var v struct {
		Scenario  string
		Base, New int
		Ratio     float64
		Functions []struct {
			Name            string
			Diff, Base, New int
		}
	}
	if err := json.Unmarshal([]byte(lines[0]), &v); err != nil {
		t.Fatalf("--json: %v\n%s", err, lines[0])
	}
	if v.Scenario != "h2" || v.Base == 0 || len(v.Functions) != 2 || v.Ratio <= 0 {
		t.Errorf("--json: %+v", v)
	}

	if code := run(t, []string{"dev", "compare", base, t.TempDir()}, &out, &errOut); code != 1 {
		t.Errorf("an empty run directory: exit %d, want 1", code)
	}
}
