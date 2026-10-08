// Package build compiles checked Xo programs: it runs the checker, the
// code generator (internal/gen), writes a Go build directory with the
// runtime (std/rt), and invokes `go build` for each target with
// CGO_ENABLED=0 (decision 0002, core 12.1).
package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/std"
)

// Target is a GOOS/GOARCH pair.
type Target struct {
	OS, Arch string
}

func (t Target) String() string { return t.OS + "/" + t.Arch }

// AllTargets are the targets of `xo build --targets=all` (core 12.1).
// `wasi/wasm32` is Go's wasip1/wasm.
var AllTargets = []Target{
	{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"},
	{"windows", "amd64"}, {"windows", "arm64"}, {"wasip1", "wasm"},
}

// Host is the target of the running toolchain.
func Host() Target { return Target{runtime.GOOS, runtime.GOARCH} }

// ParseTargets parses `all` or a comma separated list of os/arch.
func ParseTargets(s string) ([]Target, error) {
	if s == "" {
		return []Target{Host()}, nil
	}
	if s == "all" {
		return AllTargets, nil
	}
	var out []Target
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		os_, arch, ok := strings.Cut(part, "/")
		if !ok || os_ == "" || arch == "" {
			return nil, fmt.Errorf("bad target %q: want os/arch", part)
		}
		if os_ == "wasi" {
			os_ = "wasip1"
		}
		if arch == "wasm32" {
			arch = "wasm"
		}
		out = append(out, Target{os_, arch})
	}
	return out, nil
}

// Options configure a build.
type Options struct {
	Mode    gen.Mode
	Holes   bool
	Targets []Target
	// Out is the output binary path (one target) or directory (several);
	// empty means the build directory.
	Out string
	// Race builds with the race detector (host only).
	Race bool
	// Mono specializes generic code per instantiation (gen.Options.Mono).
	Mono bool
	// Stderr receives compiler output.
	Stderr *bytes.Buffer
}

// Result is the outcome of a build.
type Result struct {
	Diagnostics []diag.Diagnostic
	// Failed is set when checking reported errors (no Go was built).
	Failed bool
	// Binaries maps each target to its binary path.
	Binaries map[Target]string
	// Dir is the Go build directory.
	Dir string
	// Name is the program name (the main module's directory name).
	Name string
	// Specs counts the specialized copies of generic functions (Mono).
	Specs int
}

// Check runs the checker on dirs and reports whether errors block the
// build. Holes (XO0900 warnings) block it unless holes is set (core 3.7).
func Check(dirs []string, holes bool) (*check.Result, bool, error) {
	res, err := check.Check(dirs)
	if err != nil {
		return nil, false, err
	}
	ok := true
	for _, d := range res.Diagnostics {
		if d.Severity == diag.Error {
			ok = false
		}
		if d.Code == string(diag.Hole) && !holes {
			ok = false
		}
	}
	return res, ok, nil
}

// Build checks and compiles the modules under dirs.
func Build(dirs []string, opts Options) (*Result, error) {
	res, ok, err := Check(dirs, opts.Holes)
	if err != nil {
		return nil, err
	}
	out := &Result{Diagnostics: res.Diagnostics, Binaries: map[Target]string{}}
	if !ok {
		out.Failed = true
		return out, nil
	}
	root := lineRoot(dirs)
	prog, err := gen.Generate(res, gen.Options{Mode: opts.Mode, Holes: opts.Holes, LineRoot: root, Mono: opts.Mono})
	if err != nil {
		return nil, err
	}
	out.Specs = prog.Specs
	dir, err := buildDir(dirs, opts.Mode, opts.Mono)
	if err != nil {
		return nil, err
	}
	out.Dir = dir
	if err := writeTree(dir, prog); err != nil {
		return nil, err
	}
	name := progName(dirs)
	if opts.Mode == gen.Test {
		name += ".test"
	}
	out.Name = name
	targets := opts.Targets
	if len(targets) == 0 {
		targets = []Target{Host()}
	}
	for _, t := range targets {
		bin := binPath(dir, opts.Out, name, t, len(targets) > 1)
		if err := goBuild(dir, bin, t, opts); err != nil {
			return nil, err
		}
		out.Binaries[t] = bin
	}
	return out, nil
}

// lineRoot is the directory //line paths are relative to: the xo.mod root
// of the first directory, or the directory itself.
func lineRoot(dirs []string) string {
	if len(dirs) == 0 {
		return ""
	}
	abs, err := filepath.Abs(dirs[0])
	if err != nil {
		return ""
	}
	for d := abs; ; {
		if _, err := os.Stat(filepath.Join(d, "xo.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return abs
}

func progName(dirs []string) string {
	if len(dirs) == 0 {
		return "xoprog"
	}
	abs, err := filepath.Abs(dirs[0])
	if err != nil {
		return "xoprog"
	}
	name := filepath.Base(abs)
	if name == "." || name == "/" || name == "" {
		return "xoprog"
	}
	return name
}

func binPath(dir, out, name string, t Target, many bool) string {
	exe := ""
	if t.OS == "windows" {
		exe = ".exe"
	} else if t.OS == "wasip1" {
		exe = ".wasm"
	}
	if many {
		base := fmt.Sprintf("%s-%s-%s%s", name, t.OS, t.Arch, exe)
		if out != "" {
			return filepath.Join(out, base)
		}
		return filepath.Join(dir, "bin", base)
	}
	if out != "" {
		return out
	}
	return filepath.Join(dir, "bin", name+exe)
}

// buildDir is a stable directory per program and mode under the user
// cache, so Go's build cache is reused across builds.
func buildDir(dirs []string, mode gen.Mode, mono bool) (string, error) {
	base := os.Getenv("XO_BUILD_DIR")
	if base == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			cache = os.TempDir()
		}
		base = filepath.Join(cache, "xo", "build")
	}
	h := sha256.New()
	for _, d := range dirs {
		abs, _ := filepath.Abs(d)
		h.Write([]byte(abs + "\x00"))
	}
	fmt.Fprintf(h, "mode=%d", mode)
	if mono {
		fmt.Fprintf(h, " mono")
	}
	dir := filepath.Join(base, hex.EncodeToString(h.Sum(nil))[:16])
	return dir, os.MkdirAll(dir, 0o755)
}

// writeTree writes the generated program and the runtime into dir,
// removing generated packages left from an earlier build.
func writeTree(dir string, prog *gen.Program) error {
	if err := os.RemoveAll(filepath.Join(dir, "m")); err != nil {
		return err
	}
	files := map[string][]byte{}
	for p, src := range prog.Files {
		files[p] = src
	}
	err := fs.WalkDir(std.Runtime, "rt", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, "_test.go") {
			return err
		}
		data, err := fs.ReadFile(std.Runtime, p)
		if err != nil {
			return err
		}
		files[p] = data
		return nil
	})
	if err != nil {
		return err
	}
	// remove runtime files that no longer exist
	if ents, err := os.ReadDir(filepath.Join(dir, "rt")); err == nil {
		for _, e := range ents {
			if _, ok := files["rt/"+e.Name()]; !ok {
				os.Remove(filepath.Join(dir, "rt", e.Name()))
			}
		}
	}
	var paths []string
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if old, err := os.ReadFile(full); err == nil && bytes.Equal(old, files[p]) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, files[p], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// BuildError is a failure of the Go toolchain on generated code: a code
// generator bug.
type BuildError struct {
	Target Target
	Output string
}

func (e *BuildError) Error() string {
	return fmt.Sprintf("go build for %s failed (code generator bug):\n%s", e.Target, e.Output)
}

func goBuild(dir, bin string, t Target, opts Options) error {
	args := []string{"build", "-trimpath", "-o", bin}
	if opts.Race {
		args = append(args, "-race")
	}
	args = append(args, ".")
	cmd := exec.Command(goTool(), args...)
	cmd.Dir = dir
	env := append(os.Environ(), "GOOS="+t.OS, "GOARCH="+t.Arch, "GOWORK=off", "GOFLAGS=")
	if opts.Race {
		env = append(env, "CGO_ENABLED=1")
	} else {
		env = append(env, "CGO_ENABLED=0")
	}
	cmd.Env = env
	var outb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outb, &outb
	if err := cmd.Run(); err != nil {
		return &BuildError{Target: t, Output: outb.String()}
	}
	return nil
}

// goTool finds the go command: $XO_GO, then PATH.
func goTool() string {
	if g := os.Getenv("XO_GO"); g != "" {
		return g
	}
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	return "go"
}
