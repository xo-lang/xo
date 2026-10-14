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
func TestArchiveTree(t *testing.T) {
	root, err := Root()
	if err != nil {
		t.Skip(err)
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
