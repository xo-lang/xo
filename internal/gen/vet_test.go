package gen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// TestGeneratedGoVets builds every golden program and checks the Go it
// generates (the program's packages, not the hand written rt) with gofmt
// and go vet: the golden tests compare behavior only, so code that runs
// right but is untidy (unreachable statements, unformatted output) is
// caught here. Skipped with -short (about a minute).
func TestGeneratedGoVets(t *testing.T) {
	if testing.Short() {
		t.Skip("vets every golden program; run without -short")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		dir := dir
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			skipOffline(t, dir)
			mode := gen.Build
			if strings.HasPrefix(filepath.Base(dir), "test_") {
				mode = gen.Test
			}
			res, err := build.Build([]string{dir}, build.Options{Mode: mode})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res.Diagnostics)
			}
			pkgs := []string{"."}
			if _, err := os.Stat(filepath.Join(res.Dir, "m")); err == nil {
				pkgs = append(pkgs, "./m/...")
			}
			files := []string{}
			for _, p := range []string{"main.go", "main_test.go", "m"} {
				if _, err := os.Stat(filepath.Join(res.Dir, p)); err == nil {
					files = append(files, p)
				}
			}
			if out, err := run(res.Dir, "gofmt", append([]string{"-l"}, files...)...); err != nil || out != "" {
				t.Errorf("gofmt -l: %v\n%s", err, out)
			}
			if out, err := run(res.Dir, "go", append([]string{"vet"}, pkgs...)...); err != nil {
				t.Errorf("go vet: %v\n%s", err, out)
			}
		})
	}
}

func run(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
