package xotool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckerXo: the xo CheckerXo picks without a build has this tree's
// checker sources: its own current bin/xo, or a cached build whose
// recorded commit has the same files under CheckerDirs.
func TestCheckerXo(t *testing.T) {
	root, err := Root()
	if err != nil {
		t.Skip(err)
	}
	p, why, ok := CheckerXo(root, CheckerDirs)
	if !ok {
		t.Skip("no xo with this tree's checker sources here (the caller builds)")
	}
	if p == filepath.Join(root, "bin", exeName("xo")) {
		return
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(p), "commit"))
	if err != nil {
		t.Fatalf("%s (%s): no commit beside it", p, why)
	}
	c := strings.TrimSpace(string(b))
	args := append([]string{"-C", root, "diff", "--stat", c, "--"}, CheckerDirs...)
	out, _ := exec.Command("git", args...).Output()
	if len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("%s (%s): the checker sources differ from %s:\n%s", p, why, c, out)
	}
}

// TestUntrackedCheckerFiles: an untracked file under the checker's
// directories rules out every cached build (git diff does not see it).
func TestUntrackedCheckerFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	os.MkdirAll(filepath.Join(dir, "compiler", "check"), 0o755)
	os.WriteFile(filepath.Join(dir, "compiler", "check", "a.xo"), []byte("x"), 0o644)
	run("add", ".")
	run("commit", "-qm", "a")
	if untracked(dir, CheckerDirs) {
		t.Fatal("a clean tree has untracked checker files")
	}
	os.WriteFile(filepath.Join(dir, "compiler", "check", "b.xo"), []byte("y"), 0o644)
	if !untracked(dir, CheckerDirs) {
		t.Error("an untracked compiler/check/b.xo was not seen")
	}
}
