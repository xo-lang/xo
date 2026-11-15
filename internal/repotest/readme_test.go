package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestReadmeReleaseProof: README.md names no version number (0.NN.N)
// and no ISO date, so a release cannot leave it wrong (lead, owner
// approved 2026-11-15: it still said toolchain 0.40.0 at 0.51). Versions,
// dates and figures belong in the release notes, bench/results and
// xolang.dev, which the README links.
func TestReadmeReleaseProof(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	version := regexp.MustCompile(`\bv?\d+\.\d+\.\d+\b`)
	date := regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	for i, line := range strings.Split(string(data), "\n") {
		if m := version.FindString(line); m != "" {
			t.Errorf("README.md:%d: version %q: link the latest release instead", i+1, m)
		}
		if m := date.FindString(line); m != "" {
			t.Errorf("README.md:%d: date %q: dates go in the release notes or bench/results", i+1, m)
		}
	}
}
