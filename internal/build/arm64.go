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

// Arm64Asm lowers a checked program to SSA with reference counting and
// then to AArch64 assembly for goos ("darwin" or "linux").
func Arm64Asm(res *check.Result, root, goos string) ([]byte, error) {
	return arm64AsmOpts(res, root, goos, false)
}

// arm64AsmOpts is Arm64Asm, with every heap value marked shared as it is
// defined when probes is set (Options.ShareProbes).
func arm64AsmOpts(res *check.Result, root, goos string, probes bool) ([]byte, error) {
	asm, err := arm64Asm(res, root, goos, false, probes)
	var ie *arm64.InlineError
	if errors.As(err, &ie) {
		return arm64Asm(res, root, goos, true, probes)
	}
	return asm, err
}

func arm64Asm(res *check.Result, root, goos string, noInline, probes bool) ([]byte, error) {
	m, err := ssa.BuildOpts(res, ssa.Options{LineRoot: root})
	if err != nil {
		return nil, err
	}
	if err := m.Verify(); err != nil {
		return nil, err
	}
	if probes {
		m.InsertShareProbes()
	}
	m.InsertRC()
	if err := m.Verify(); err != nil {
		return nil, err
	}
	place, err := arm64.ParseLayout(os.Getenv("XO_ARM64_LAYOUT"))
	if err != nil {
		return nil, fmt.Errorf("XO_ARM64_LAYOUT: %v", err)
	}
	prog, err := arm64.Generate(m, arm64.Options{OS: arm64.OS(goos), NoInline: noInline, Layout: place})
	if err != nil {
		return nil, err
	}
	return prog.Asm, nil
}

// buildArm64 generates assembly for each target; the host target is
// assembled and linked with cc and the C runtime (std/llrt), other
// arm64 targets are written as assembly only.
func buildArm64(dirs []string, res *check.Result, out *Result, opts Options) (*Result, error) {
	if opts.Mode != gen.Build {
		return nil, fmt.Errorf("the arm64 backend builds programs only (not tests)")
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
	dir, err := buildDir(dirs, opts.Mode, false, opts.NoRanges, false)
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
	out.Name = name
	root := lineRoot(dirs)
	for _, t := range ts {
		asm, err := arm64AsmOpts(res, root, t.OS, opts.ShareProbes)
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
		rtSrc, err := fs.ReadFile(std.LLRuntime, "llrt/xo_rt.c")
		if err != nil {
			return nil, err
		}
		rt := filepath.Join(dir, "xo_rt.c")
		if err := os.WriteFile(rt, rtSrc, 0o644); err != nil {
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
		if err := ccBuild(cc, []string{"-O2"}, []string{s, rt}, bin); err != nil {
			return nil, err
		}
		out.Binaries[t] = bin
	}
	return out, nil
}
