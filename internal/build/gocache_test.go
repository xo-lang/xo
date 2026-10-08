package build

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrimGoCacheKeepsRecent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XO_GOCACHE", dir)
	t.Setenv("XO_GOCACHE_MAX", "100")
	now := time.Now()
	files := []struct {
		name string
		age  time.Duration
		kept bool
	}{
		{"old1", 5 * time.Hour, false},
		{"old2", 4 * time.Hour, false},
		{"recent1", 30 * time.Minute, true},
		{"recent2", time.Minute, true},
	}
	for _, f := range files {
		p := filepath.Join(dir, f.name)
		if err := os.WriteFile(p, make([]byte, 60), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := now.Add(-f.age)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	trimGoCache()
	for _, f := range files {
		_, err := os.Stat(filepath.Join(dir, f.name))
		if kept := err == nil; kept != f.kept {
			t.Errorf("%s: kept=%v, want %v", f.name, kept, f.kept)
		}
	}
}
