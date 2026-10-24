//go:build unix

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

// TestBuildDevNull: `xo build -o /dev/null` builds and writes the program
// to the device: /dev takes no stage directory, so the program is staged
// in the user cache and its bytes written to the output (dfd47760e's
// stages had broken the idiom: "cannot write the program to /dev/null").
func TestBuildDevNull(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/null")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
	})
	for _, be := range []string{"", "--backend=go"} {
		args := []string{"build", "-o", "/dev/null"}
		if be != "" {
			args = append(args, be)
		}
		var out, errb bytes.Buffer
		if code := run(t, append(args, filepath.Join(dir, "app")), &out, &errb); code != 0 {
			t.Errorf("xo build %s -o /dev/null: exit %d\n%s%s", be, code, out.String(), errb.String())
		}
	}
}

// TestBuildWorkDirsPrivate: the files of a build (assembly, part objects)
// are in a directory of that build, so concurrent builds of one program
// never write the file another links (two concurrent builds failed once
// with "prog-darwin.1.o: not a 64 bit Mach-O file", 2026-10-24); the
// directory is gone after the link, and XO_KEEP_ASM=1 keeps the assembly
// beside the build directory's other files.
func TestBuildWorkDirsPrivate(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
		"out/.keep":   "",
	})
	chdir(t, filepath.Join(dir, "app"))
	setenv(t, "XO_BIN_CACHE", "off")
	setenv(t, "XO_WHOLE_BUILD", "off")
	bdir := filepath.Join(dir, "build")
	setenv(t, "XO_BUILD_DIR", bdir)
	for round := 0; round < 4; round++ {
		var wg sync.WaitGroup
		codes := make([]int, 4)
		errs := make([]string, 4)
		for i := range codes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var out, errOut bytes.Buffer
				codes[i] = run(t, []string{"build", "-o", filepath.Join(dir, "out", "p"+strconv.Itoa(i)), "."}, &out, &errOut)
				errs[i] = errOut.String()
			}()
		}
		wg.Wait()
		for i, c := range codes {
			if c != 0 {
				t.Errorf("round %d build %d: exit %d\n%s", round, i, c, errs[i])
			}
		}
	}
	if left, _ := filepath.Glob(filepath.Join(bdir, "*", "*", "w-*")); len(left) > 0 {
		t.Errorf("work directories left after the links: %v", left)
	}
	if kept, _ := filepath.Glob(filepath.Join(bdir, "*", "*", "prog-*.o")); len(kept) > 0 && runtime.GOARCH == "arm64" {
		t.Errorf("part objects left in the build directory: %v", kept)
	}

	setenv(t, "XO_KEEP_ASM", "1")
	var out, errOut bytes.Buffer
	if c := run(t, []string{"build", "-o", filepath.Join(dir, "out", "keep"), "."}, &out, &errOut); c != 0 {
		t.Fatalf("XO_KEEP_ASM build: exit %d\n%s", c, errOut.String())
	}
	if kept, _ := filepath.Glob(filepath.Join(bdir, "*", "*", "prog-*")); len(kept) == 0 {
		t.Errorf("XO_KEEP_ASM=1 kept no prog-* file in the build directory")
	}
}
