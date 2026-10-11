package build

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrimBuildDirs(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XO_BUILD_DIR_MAX", "3000")
	old := time.Now().Add(-48 * time.Hour)
	mk := func(name string, mod time.Time) string {
		d := filepath.Join(base, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "f"), make([]byte, 1000), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(d, mod, mod); err != nil {
			t.Fatal(err)
		}
		return d
	}
	oldest := mk("a", old.Add(-time.Hour))
	older := mk("b", old)
	recent1 := mk("c", time.Now())
	recent2 := mk("d", time.Now())
	trimBuildDirs(base)
	for _, d := range []string{oldest, older} {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("%s kept, want removed (least recently used)", d)
		}
	}
	for _, d := range []string{recent1, recent2} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("%s removed, want kept (used within keepRecent)", d)
		}
	}
}
