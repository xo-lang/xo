package testtmp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestSweep(t *testing.T) {
	prefix := "xo-testtmp-" + strconv.Itoa(os.Getpid()) + "-"
	t.Cleanup(func() {
		m, _ := filepath.Glob(filepath.Join(os.TempDir(), prefix+"*"))
		for _, d := range m {
			os.RemoveAll(d)
		}
	})

	// a finished process: its PID no longer exists
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Skip(err)
	}
	mk := func(name, owner string, age time.Duration) string {
		d := filepath.Join(os.TempDir(), prefix+name)
		if err := os.MkdirAll(filepath.Join(d, "build"), 0o755); err != nil {
			t.Fatal(err)
		}
		if owner != "" {
			os.WriteFile(filepath.Join(d, ownerFile), []byte(owner), 0o644)
		}
		when := time.Now().Add(-age)
		os.Chtimes(d, when, when)
		return d
	}
	dead := mk("dead", strconv.Itoa(cmd.Process.Pid), 0)
	live := mk("live", strconv.Itoa(os.Getppid()), 0)
	newUnowned := mk("new", "", time.Minute)
	oldUnowned := mk("old", "", 2*unownedAge)

	mine, err := Dir(prefix)
	if err != nil {
		t.Fatal(err)
	}
	Sweep(prefix)

	for _, c := range []struct {
		dir  string
		want bool
	}{{dead, false}, {live, true}, {newUnowned, true}, {oldUnowned, false}, {mine, true}} {
		_, err := os.Stat(c.dir)
		if got := err == nil; got != c.want {
			t.Errorf("%s exists = %v, want %v", filepath.Base(c.dir), got, c.want)
		}
	}
}
