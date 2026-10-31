package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseLicenses runs scripts/release-licenses.sh (decision 0302,
// point 4) on staged trees built from the repository's
// THIRD_PARTY_NOTICES: a stage with no race runtime passes, one with
// lib/xo/race and its LICENSE.TXT passes, and one with lib/xo/race but
// no LICENSE.TXT fails naming the compiler-rt item.
func TestReleaseLicenses(t *testing.T) {
	notices, err := os.ReadFile(filepath.Join(root, "THIRD_PARTY_NOTICES"))
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "scripts", "release-licenses.sh")
	stage := func(race, license bool) string {
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "THIRD_PARTY_NOTICES"), notices, 0o644); err != nil {
			t.Fatal(err)
		}
		if race {
			if err := os.MkdirAll(filepath.Join(d, "lib", "xo", "race", "darwin-arm64"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if license {
			if err := os.WriteFile(filepath.Join(d, "lib", "xo", "race", "LICENSE.TXT"), []byte("license"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return d
	}
	for _, c := range []struct {
		name          string
		race, license bool
		ok            bool
	}{
		{"no race runtime", false, false, true},
		{"race runtime with its license", true, true, true},
		{"race runtime without its license", true, false, false},
	} {
		out, err := exec.Command("bash", script, stage(c.race, c.license)).CombinedOutput()
		if c.ok && err != nil {
			t.Errorf("%s: release-licenses.sh failed: %v\n%s", c.name, err, out)
		}
		if !c.ok && (err == nil || !strings.Contains(string(out), "compiler-rt")) {
			t.Errorf("%s: release-licenses.sh passed or did not name the compiler-rt item: %v\n%s", c.name, err, out)
		}
	}
}
