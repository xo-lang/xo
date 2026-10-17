package repotest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestNoConflictMarkers fails when a tracked text file holds a merge
// conflict's markers (a line starting `<<<<<<< ` or `>>>>>>> `, or a line
// of exactly `=======`): a `git pull --rebase` that stopped on a conflict
// in a script's quiet pipeline left them in CHANGELOG.md and a gate ran
// on the half finished rebase (gotchas/git-pull-rebase-stops-silently.md).
// Files with a NUL byte are binary and skipped.
func TestNoConflictMarkers(t *testing.T) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("git ls-files: %v", err)
	}
	for _, name := range bytes.Split(out, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		path := filepath.Join(root, string(name))
		data, err := os.ReadFile(path)
		if err != nil || bytes.IndexByte(data, 0) >= 0 {
			// deleted in the worktree, or binary
			continue
		}
		for i, line := range bytes.Split(data, []byte("\n")) {
			line = bytes.TrimSuffix(line, []byte("\r"))
			if bytes.HasPrefix(line, []byte("<<<<<<< ")) || bytes.HasPrefix(line, []byte(">>>>>>> ")) || bytes.Equal(line, []byte("=======")) {
				t.Errorf("%s:%d: a merge conflict's marker %q (finish or abort the rebase)", name, i+1, line)
			}
		}
	}
}
