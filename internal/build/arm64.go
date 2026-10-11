package build

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/xo-lang/xo/internal/arm64"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/ssa"
	"github.com/xo-lang/xo/std"
)

// BackendArm64 is the native AArch64 backend (decision 0049).
const BackendArm64 = "arm64"

// ErrNoCC reports that the arm64 backend found no C compiler.
var ErrNoCC = errors.New("the arm64 backend needs a C compiler to assemble and link: install one or set XO_CC")

// CC finds the C compiler that assembles and links arm64 builds: $XO_CC,
// then cc, then clang.
func CC() (string, error) {
	if c := os.Getenv("XO_CC"); c != "" {
		p, err := exec.LookPath(c)
		if err != nil {
			return "", fmt.Errorf("%w (XO_CC=%s: %v)", ErrNoCC, c, err)
		}
		return p, nil
	}
	for _, c := range []string{"cc", "clang"} {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", ErrNoCC
}

// writeRuntime writes the C runtime of the native backends (std/llrt:
// xo_rt.c and the files it includes) into dir and returns the path of
// xo_rt.c, the one file to compile.
func writeRuntime(dir string) (string, error) {
	files, err := fs.Glob(std.LLRuntime, "llrt/*.c")
	if err != nil {
		return "", err
	}
	for _, f := range files {
		src, err := fs.ReadFile(std.LLRuntime, f)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(f)), src, 0o644); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "xo_rt.c"), nil
}

// Arm64Asm lowers a checked program to SSA with reference counting and
// then to AArch64 assembly for goos ("darwin" or "linux").
func Arm64Asm(res *check.Result, root, goos string) ([]byte, error) {
	asm, _, err := arm64AsmOpts(res, root, goos, "", false, false, false, false)
	return asm, err
}

// arm64AsmOpts is Arm64Asm, with every heap value marked shared as it is
// defined when probes is set (Options.ShareProbes), debug contracts
// compiled out when release is set. It also returns the zone data
// std/time needs.
func arm64AsmOpts(res *check.Result, root, goos, layout string, probes, tests, realTests, release bool) ([]byte, ssa.TZUse, error) {
	asm, tz, err := arm64Asm(res, root, goos, layout, false, probes, tests, realTests, release)
	var ie *arm64.InlineError
	if errors.As(err, &ie) {
		return arm64Asm(res, root, goos, layout, true, probes, tests, realTests, release)
	}
	return asm, tz, err
}

func arm64Asm(res *check.Result, root, goos, layout string, noInline, probes, tests, realTests, release bool) ([]byte, ssa.TZUse, error) {
	var tz ssa.TZUse
	m, err := ssa.BuildOpts(res, ssa.Options{LineRoot: root, Tests: tests, RealTests: realTests, Release: release, GOOS: goos, GOARCH: "arm64"})
	if err != nil {
		return nil, tz, err
	}
	tz = m.TZ
	if err := m.Verify(); err != nil {
		return nil, tz, err
	}
	if probes {
		m.InsertShareProbes()
	}
	m.InsertRC()
	if err := m.Verify(); err != nil {
		return nil, tz, err
	}
	if layout == "" {
		layout = os.Getenv("XO_ARM64_LAYOUT")
	}
	place, err := arm64.ParseLayout(layout)
	if err != nil {
		return nil, tz, fmt.Errorf("XO_ARM64_LAYOUT: %v", err)
	}
	prog, err := arm64.Generate(m, arm64.Options{OS: arm64.OS(goos), NoInline: noInline, Layout: place})
	if err != nil {
		return nil, tz, err
	}
	return prog.Asm, tz, nil
}

// buildArm64 generates assembly for each target; the host target is
// assembled and linked with cc and the C runtime (std/llrt), other
// arm64 targets are written as assembly only.
func buildArm64(dirs []string, res *check.Result, out *Result, opts Options) (*Result, error) {
	if opts.Mode != gen.Build && opts.Mode != gen.Test {
		return nil, fmt.Errorf("the arm64 backend builds programs and tests only")
	}
	if opts.Race || opts.Holes {
		return nil, fmt.Errorf("the arm64 backend does not support --race or --holes")
	}
	ts := opts.Targets
	if len(ts) == 0 {
		ts = []Target{Host()}
	}
	for _, t := range ts {
		if t.Arch != "arm64" || (t.OS != "darwin" && t.OS != "linux") {
			return nil, fmt.Errorf("the arm64 backend builds for darwin/arm64 and linux/arm64, not %s", t)
		}
	}
	variants := []string{releaseVariant(opts)}
	if opts.ARM64Layout != "" {
		variants = append(variants, "layout "+opts.ARM64Layout)
	}
	dir, err := buildDir(dirs, opts.Mode, false, opts.NoRanges, false, variants...)
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "arm64")
	if opts.ShareProbes {
		dir += "-share"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	out.Dir = dir
	out.Asm = map[Target]string{}
	name := progName(dirs)
	if opts.Mode == gen.Test {
		name += ".test"
	}
	out.Name = name
	root := lineRoot(dirs)
	for _, t := range ts {
		asm, tz, err := arm64AsmOpts(res, root, t.OS, opts.ARM64Layout, opts.ShareProbes, opts.Mode == gen.Test, opts.RealTests, opts.release())
		if err != nil {
			return nil, err
		}
		s := filepath.Join(dir, "prog-"+t.OS+".s")
		if err := os.WriteFile(s, asm, 0o644); err != nil {
			return nil, err
		}
		out.Asm[t] = s
		if t != Host() {
			continue
		}
		rt, err := writeRuntime(dir)
		if err != nil {
			return nil, err
		}
		cc, err := CC()
		if err != nil {
			return nil, err
		}
		bin := binPath(dir, opts.Out, name, t, false)
		if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
			return nil, err
		}
		flags, tzsrcs, notes, err := timeFlags(dir, res, tz, opts)
		if err != nil {
			return nil, err
		}
		flags = append(flags, IOFlags(asm)...)
		out.Notes = notes
		csrcs, cflags, err := cInputs(res, dir)
		if err != nil {
			return nil, err
		}
		ccFlags, srcs := append(append([]string{"-O2"}, flags...), cflags...), append(append([]string{s, rt}, tzsrcs...), csrcs...)
		key := nativeBinKey(dir, cc, append([]string{"arm64"}, ccFlags...), srcs)
		if err := withBinCache(key, bin, func() error { return ccBuild(cc, ccFlags, srcs, bin) }); err != nil {
			return nil, err
		}
		out.Binaries[t] = bin
	}
	return out, nil
}
