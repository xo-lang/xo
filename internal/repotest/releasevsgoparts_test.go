package repotest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseVsGoParts checks scripts/release-vs-go.sh's RVG_PART (task
// #645): the compile section splits into M parts, run as M leases of 30
// minutes or less on host 1, and the M parts together select every row of
// the whole section exactly once, the warm-memory notes in the same part as
// coll's run-cache-off row they build in. RVG_LIST=1 lists a part's rows
// without building, so a fake xo serves.
func TestReleaseVsGoParts(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go")
	}
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "xo")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'xo 0.0.0-fake'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "scripts", "release-vs-go.sh")
	n := 0
	list := func(part string) ([]string, int) {
		t.Helper()
		n++
		cmd := exec.Command("bash", script, fake, filepath.Join(tmp, fmt.Sprintf("out%d", n)))
		cmd.Env = append(os.Environ(), "SECTIONS=compile", "RVG_LIST=1", "RVG_PART="+part)
		out, err := cmd.CombinedOutput()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		var rows []string
		for _, l := range strings.Split(string(out), "\n") {
			if r, ok := strings.CutPrefix(l, "rvg-row "); ok {
				rows = append(rows, r)
			}
		}
		return rows, code
	}
	whole, code := list("1/1")
	if code != 0 || len(whole) < 20 {
		t.Fatalf("RVG_PART=1/1: exit %d, %d rows %q", code, len(whole), whole)
	}
	want := map[string]int{}
	for _, r := range whole {
		want[r]++
		if want[r] > 1 {
			t.Fatalf("row %q twice in the whole section; the parts could not tell them apart", r)
		}
	}
	for _, m := range []int{2, 3, 4} {
		got := map[string]int{}
		for k := 1; k <= m; k++ {
			rows, code := list(fmt.Sprintf("%d/%d", k, m))
			if code != 0 {
				t.Fatalf("RVG_PART=%d/%d: exit %d", k, m, code)
			}
			hasWarm, hasColl := false, false
			for _, r := range rows {
				got[r]++
				hasWarm = hasWarm || r == "warm_mem"
				hasColl = hasColl || r == "xo coll nocache"
			}
			if hasWarm != hasColl {
				t.Errorf("RVG_PART=%d/%d: warm_mem %v but coll's run-cache-off row %v; the notes build in that row's copy", k, m, hasWarm, hasColl)
			}
			if limit := (len(whole)+m-1)/m + 1; len(rows) > limit {
				t.Errorf("RVG_PART=%d/%d: %d rows, more than an even share (%d)", k, m, len(rows), limit)
			}
		}
		for r := range want {
			if got[r] != 1 {
				t.Errorf("M=%d: row %q in %d parts, want 1", m, r, got[r])
			}
		}
		for r := range got {
			if want[r] == 0 {
				t.Errorf("M=%d: row %q is in no whole section", m, r)
			}
		}
	}
	for _, bad := range []string{"0/2", "3/2", "x", "1/0"} {
		if _, code := list(bad); code != 2 {
			t.Errorf("RVG_PART=%s: exit %d, want 2", bad, code)
		}
	}
}
