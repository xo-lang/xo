package rt

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCrashDir: bundles default to <user cache>/xo/crash (owner,
// 2026-10-22), which is made when missing, and the oldest past the limit
// are removed, other files left alone.
func TestCrashDir(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache) // Linux
	t.Setenv("HOME", cache)           // darwin: $HOME/Library/Caches
	base, err := os.UserCacheDir()
	if err != nil {
		t.Skip(err)
	}
	want := filepath.Join(base, "xo", "crash")
	if got := crashDir(); got != want {
		t.Fatalf("crashDir() = %s, want %s", got, want)
	}
	if st, err := os.Stat(want); err != nil || !st.IsDir() {
		t.Fatalf("%s not made: %v", want, err)
	}
	now := time.Now()
	for i := 0; i < 5; i++ {
		p := filepath.Join(want, fmt.Sprintf("prog-%d.xocrash", i))
		os.WriteFile(p, []byte("{}"), 0o644)
		os.Chtimes(p, now, now.Add(time.Duration(i)*time.Second))
	}
	os.WriteFile(filepath.Join(want, "notes.txt"), []byte("x"), 0o644)
	pruneCrashBundles(want, 2)
	for i := 0; i < 5; i++ {
		_, err := os.Stat(filepath.Join(want, fmt.Sprintf("prog-%d.xocrash", i)))
		if kept := err == nil; kept != (i >= 3) {
			t.Errorf("prog-%d.xocrash kept %v", i, kept)
		}
	}
	if _, err := os.Stat(filepath.Join(want, "notes.txt")); err != nil {
		t.Errorf("a file not a bundle was removed")
	}
}
