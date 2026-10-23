package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdVersionMatches: std/VERSION, the stamp an xo compares with its own
// version (compiler/xo provenance.xo stale_std_note), is the toolchain's
// version (compiler/cli/core VERSION); scripts/seed-cut.sh bumps both.
func TestStdVersionMatches(t *testing.T) {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	cli, err := os.ReadFile(filepath.Join(root, "compiler", "cli", "core", "cli.xo"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^pub const VERSION = "([^"]+)"$`).FindSubmatch(cli)
	if m == nil {
		t.Fatal("no VERSION in compiler/cli/core/cli.xo")
	}
	sv, err := os.ReadFile(filepath.Join(root, "std", "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(sv)); got != string(m[1]) {
		t.Errorf("std/VERSION is %q, the toolchain is %q: bump them together (scripts/seed-cut.sh)", got, m[1])
	}
}
