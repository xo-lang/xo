package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestParallelGoBuilds starts eight `xo build --backend=go` of one program
// at once, three times: they share the Go backend's build tree, which each
// build rewrites, so without the tree lock (compiler/xo gobackend.xo) one
// build deleted the files another was formatting or compiling (150 of 160
// such builds failed, 2026-10-30: "rt: directory not empty", "main.go: no
// such file or directory", "size of xo_adapt.go changed during reading").
func TestParallelGoBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("eight Go builds at once, three times")
	}
	t.Parallel()
	dir := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(dir, ".build"))
	setenv(t, "XO_WORK", "off")
	app := filepath.Join(dir, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":  "module example.com/par\nedition preview\n",
		"main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
	})
	const jobs = 8
	for round := 0; round < 3; round++ {
		var wg sync.WaitGroup
		fails := make([]string, jobs)
		for j := 0; j < jobs; j++ {
			wg.Add(1)
			go func(j int) {
				defer wg.Done()
				var out, errOut bytes.Buffer
				bin := filepath.Join(dir, fmt.Sprintf("bin%d", j))
				if code := run(t, []string{"build", "--backend=go", "-o", bin, app}, &out, &errOut); code != 0 {
					fails[j] = fmt.Sprintf("exit %d\n%s%s", code, out.String(), errOut.String())
				}
			}(j)
		}
		wg.Wait()
		for j, f := range fails {
			if f != "" {
				t.Errorf("round %d build %d: %s", round, j, f)
			}
		}
	}
}
