package selfhost

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmbedComplete checks that Files holds every .xo file of the module
// on disk: a package one directory deeper than the embed patterns reach
// builds from the repository but not from the embedded sources
// (internal/selfdriver builds xo1 from them).
func TestEmbedComplete(t *testing.T) {
	n := 0
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".xo") {
			return nil
		}
		n++
		want, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		got, err := fs.ReadFile(Files, filepath.ToSlash(p))
		if err != nil {
			t.Errorf("%s is not embedded: %v", p, err)
			return nil
		}
		if string(got) != string(want) {
			t.Errorf("%s: the embedded copy differs", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no .xo files found")
	}
}
