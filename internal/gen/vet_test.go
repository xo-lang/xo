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
			vetProgram(t, dir, mode)
		})
	}
}

// TestGeneratedGoVetsLarge is TestGeneratedGoVets over the larger
// programs: the examples, the LLVM backend's golden programs, the perf
// benchmarks, and the self hosted compiler (built as its test binary
// when it has no main). A program only native builds can compile (`use
// c`, XO1201) is skipped; bench/paper is not included (blind solutions
// to older spec versions, which no longer check). Skipped with -short.
func TestGeneratedGoVetsLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("vets the larger programs; run without -short")
	}
	var dirs []string
	for _, pat := range []string{
		"../../examples/*",
		"../llvm/testdata/golden/*",
		"../../bench/perf/llvm/xo/*",
		"../../selfhost/*",
	} {
		ds, err := filepath.Glob(pat)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range ds {
			if xs, _ := filepath.Glob(filepath.Join(d, "*.xo")); len(xs) > 0 {
				dirs = append(dirs, d)
			}
		}
	}
	if len(dirs) < 60 {
		t.Fatalf("only %d programs found", len(dirs))
	}
	for _, dir := range dirs {
		dir := dir
		name := strings.TrimPrefix(filepath.ToSlash(dir), "../")
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			t.Parallel()
			skipOffline(t, dir)
			mode := gen.Build
			if res, _, err := build.Generate([]string{dir}, build.Options{Mode: gen.Build}); err != nil && strings.Contains(err.Error(), "no `fn main`") || res != nil && res.Failed {
				mode = gen.Test
			}
			vetProgram(t, dir, mode)
		})
	}
}

// vetProgram builds the program in dir and runs gofmt -l and go vet on
// its generated packages (not the hand written rt).
func vetProgram(t *testing.T, dir string, mode gen.Mode) {
	t.Helper()
	res, err := build.Build([]string{dir}, build.Options{Mode: mode})
	if res != nil && res.Failed {
		for _, d := range res.Diagnostics {
			if d.Code == "XO1201" {
				t.Skip("native builds only: " + d.Message)
			}
		}
	}
	if err != nil || res.Failed {
		var ds []string
		if res != nil {
			for _, d := range res.Diagnostics {
				ds = append(ds, d.Code+" "+d.Message)
			}
		}
		t.Fatalf("build: %v %v", err, ds)
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
}

func run(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if name == "go" {
		cmd.Env = goVetEnv()
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// goVetEnv is the environment of a go command run in a build directory.
// Each program's build directory has its own copy of rt, and without
// -trimpath that directory is part of the cache key, so every go vet
// compiled rt again (24 MB per program); the nightly TestArithProperty
// wrote 100 GB into the default Go cache in one night. -trimpath leaves
// the directory out of the key, and the Xo Go cache is capped
// (gotchas/go-vet-recompiles-rt-per-build-dir.md).
func goVetEnv() []string {
	return append(os.Environ(), "GOWORK=off", "GOFLAGS=-trimpath", "GOCACHE="+build.GoCacheDir())
}
