package build

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
)

// TestCopyFileNoTextBusy copies a program with copyFile (the bin cache's
// fallback across file systems) and runs the copy at once while other
// goroutines fork and exec in a loop, as parallel golden tests do. Before
// copyFile held syscall.ForkLock, a child forked during the copy kept the
// write descriptor until its exec, and running the copy failed with
// ETXTBSY ("text file busy") on Linux (2026-10-12).
func TestCopyFileNoTextBusy(t *testing.T) {
	prog, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no true(1)")
	}
	dir := t.TempDir()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				exec.Command(prog).Run()
			}
		}()
	}
	defer func() {
		close(stop)
		wg.Wait()
	}()
	n := 300
	if testing.Short() {
		n = 100
	}
	for i := 0; i < n; i++ {
		dst := filepath.Join(dir, fmt.Sprintf("p%d", i))
		if err := copyFile(prog, dst); err != nil {
			t.Fatal(err)
		}
		if err := exec.Command(dst).Run(); err != nil {
			if errors.Is(err, syscall.ETXTBSY) {
				t.Fatalf("copy %d: %v", i, err)
			}
			t.Fatalf("run copy %d: %v", i, err)
		}
	}
}
