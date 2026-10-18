package repotest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// testBlock is the start of a `test "..."` block or property test.
var testBlock = regexp.MustCompile(`^\s*test\s+"`)

// TestTestsInTestFiles keeps a convention of this repository (decision
// 0262 item 5, owner 2026-10-18; user code keeps inline tests): under
// std/ and compiler/, `test` blocks live only in _test.xo files, so a test
// change never changes xo's source key (internal/xotool Inputs skips
// *_test.xo) or costs a rebuild.
func TestTestsInTestFiles(t *testing.T) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", "std", "compiler").Output()
	if err != nil {
		t.Skipf("git ls-files: %v", err)
	}
	for _, name := range bytes.Split(out, []byte{0}) {
		n := string(name)
		if !strings.HasSuffix(n, ".xo") || strings.HasSuffix(n, "_test.xo") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, n))
		if err != nil {
			// deleted in the worktree
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			if testBlock.MatchString(line) {
				t.Errorf("%s:%d: a test block outside a _test.xo file; move it to %s_test.xo beside it", n, i+1, strings.TrimSuffix(filepath.Base(n), ".xo"))
			}
		}
	}
}
