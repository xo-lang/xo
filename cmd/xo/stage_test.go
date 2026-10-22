//go:build unix

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestBuildStagedOutputs covers the hidden staging of final outputs
// (decision 0345's output paths, owner 2026-10-22): two concurrent
// `xo build -o same` both succeed and leave nothing but the program and
// its debug information; no visible file appears in the output
// directory while they build (a poll every 2 ms); a SIGINT during a
// build removes its hidden stage.
func TestBuildStagedOutputs(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
		"out/.keep":   "",
	})
	chdir(t, filepath.Join(dir, "app"))
	setenv(t, "XO_BIN_CACHE", "off")
	outDir := filepath.Join(dir, "out")
	final := filepath.Join(outDir, "prog")
	// visible names other than the outputs, seen while building
	var mu sync.Mutex
	var seen []string
	stop := make(chan struct{})
	polled := make(chan struct{})
	go func() {
		defer close(polled)
		for {
			select {
			case <-stop:
				return
			default:
			}
			ents, _ := os.ReadDir(outDir)
			for _, e := range ents {
				n := e.Name()
				if !strings.HasPrefix(n, ".") && n != "prog" && n != "prog.dSYM" {
					mu.Lock()
					seen = append(seen, n)
					mu.Unlock()
				}
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	var wg sync.WaitGroup
	codes := make([]int, 2)
	errs := make([]string, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out, errOut bytes.Buffer
			codes[i] = run(t, []string{"build", "-o", final, "."}, &out, &errOut)
			errs[i] = errOut.String()
		}()
	}
	wg.Wait()
	close(stop)
	<-polled
	for i := range 2 {
		if codes[i] != 0 {
			t.Errorf("build %d: exit %d\n%s", i, codes[i], errs[i])
		}
	}
	if len(seen) > 0 {
		t.Errorf("visible files during the builds: %v", seen)
	}
	ents, _ := os.ReadDir(outDir)
	for _, e := range ents {
		if n := e.Name(); n != ".keep" && n != "prog" && n != "prog.dSYM" {
			t.Errorf("left in the output directory: %s", n)
		}
	}
	if fi, err := os.Stat(final); err != nil || fi.Mode()&0o111 == 0 {
		t.Errorf("no executable %s: %v", final, err)
	}

	// a SIGINT while a stage exists: the stage goes, xo exits 130
	cli, err := xoCommand()
	if err != nil {
		t.Fatal(err)
	}
	for try := 0; try < 5; try++ {
		cmd := command(t, cli[0], append(append([]string{}, cli[1:]...), "build", "-o", filepath.Join(outDir, "int"), ".")...)
		var errOut bytes.Buffer
		cmd.Stderr = &errOut
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		staged := false
		for i := 0; i < 5000 && !staged; i++ {
			ms, _ := filepath.Glob(filepath.Join(outDir, ".int.tmp-*"))
			staged = len(ms) > 0
			if !staged {
				time.Sleep(time.Millisecond)
			}
		}
		if staged {
			cmd.Process.Signal(syscall.SIGINT) // the process this test started
		}
		cmd.Wait()
		ms, _ := filepath.Glob(filepath.Join(outDir, ".int.tmp-*"))
		if len(ms) > 0 {
			t.Errorf("a SIGINT left %v", ms)
		}
		if staged && cmd.ProcessState.ExitCode() == 130 {
			return
		}
		os.Remove(filepath.Join(outDir, "int"))
		os.RemoveAll(filepath.Join(outDir, "int.dSYM"))
	}
	t.Log("the SIGINT never fell inside a stage in 5 tries (the link was too quick); only the no-leftover checks ran")
}
