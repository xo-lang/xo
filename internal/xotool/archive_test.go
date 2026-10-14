package xotool

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestArchiveTree unpacks a commit of this repository with archiveTree
// (archive/tar, no external tar: gotchas/xotool-tar-pipe-windows.md).
// With -short the commit is of a small repository holding the files the
// test looks at: the whole checkout (78 MB) took 1.3 to 7.6 s.
func TestArchiveTree(t *testing.T) {
	t.Parallel()
	root, err := Root()
	if err != nil {
		t.Skip(err)
	}
	if testing.Short() {
		root = smallRepo(t)
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Run(); err != nil {
		t.Skip("not a git checkout")
	}
	dir := t.TempDir()
	if err := archiveTree(root, "HEAD", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatalf("go.mod not unpacked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "selfhost", "xo", "main.xo")); err != nil {
		t.Fatalf("selfhost/xo/main.xo not unpacked: %v", err)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dir, "scripts", "bootstrap-xo.sh"))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode()&0o111 == 0 {
			t.Errorf("scripts/bootstrap-xo.sh lost its executable bit: %v", fi.Mode())
		}
	}
	// a commit that does not exist fails without hanging
	if err := archiveTree(root, "0000000000000000000000000000000000000000", t.TempDir()); err == nil {
		t.Error("archiveTree of a missing commit succeeded")
	}
}

// smallRepo is a git repository of one commit with the files
// TestArchiveTree checks (scripts/bootstrap-xo.sh executable).
func smallRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	for name, mode := range map[string]os.FileMode{"go.mod": 0o644, "selfhost/xo/main.xo": 0o644, "scripts/bootstrap-xo.sh": 0o755} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(name+"\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "small"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=xo", "GIT_AUTHOR_EMAIL=xo@example.invalid",
			"GIT_COMMITTER_NAME=xo", "GIT_COMMITTER_EMAIL=xo@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}
