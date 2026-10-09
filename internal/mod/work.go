package mod

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
)

// Work is a parsed xo.work workspace (decision 0086): the modules it
// `use`s are built from their local directories, overriding every
// `require` of them.
type Work struct {
	File    string
	Dir     string
	Modules []*File // the used modules, in file order
}

// WorkOff reports whether workspaces are turned off: XO_WORK=off (set by
// `--workspace=off`).
func WorkOff() bool { return os.Getenv("XO_WORK") == "off" }

// FindWork returns the workspace that applies to the module in dir: the
// file named by XO_WORK, or the nearest xo.work at or above dir. nil when
// there is none, when XO_WORK=off, or when the workspace does not use the
// module in dir.
func FindWork(dir string) (*Work, []*Problem) {
	if WorkOff() {
		return nil, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil
	}
	name := ""
	if env := os.Getenv("XO_WORK"); env != "" {
		name, _ = filepath.Abs(env)
	} else {
		for d := abs; ; {
			if _, err := os.Stat(filepath.Join(d, "xo.work")); err == nil {
				name = filepath.Join(d, "xo.work")
				break
			}
			parent := filepath.Dir(d)
			if parent == d {
				return nil, nil
			}
			d = parent
		}
	}
	w, probs := ReadWork(name)
	if w == nil {
		return nil, probs
	}
	for _, m := range w.Modules {
		if m.Dir == abs {
			return w, probs
		}
	}
	return nil, nil
}

// ReadWork parses an xo.work file:
//
//	xo 2026
//	use ./api
//	use ../shared-lib
func ReadWork(name string) (*Work, []*Problem) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, []*Problem{{Code: diag.ModFileInvalid, File: name, Msg: fmt.Sprintf("cannot read xo.work: %v", err)}}
	}
	w := &Work{File: name, Dir: filepath.Dir(name)}
	var probs []*Problem
	seen := map[string]int{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	line := 0
	for sc.Scan() {
		line++
		text := sc.Text()
		if i := strings.Index(text, "//"); i >= 0 {
			text = text[:i]
		}
		f := strings.Fields(text)
		if len(f) == 0 {
			continue
		}
		switch {
		case f[0] == "xo" && len(f) == 2:
		case f[0] == "use" && len(f) == 2:
			dir := filepath.Clean(filepath.Join(w.Dir, filepath.FromSlash(f[1])))
			if filepath.IsAbs(f[1]) {
				dir = filepath.Clean(f[1])
			}
			mf, ps := ReadFile(dir)
			if mf == nil || mf.Path == "" {
				probs = append(probs, &Problem{Code: diag.ModFileInvalid, File: name, Line: line, Msg: fmt.Sprintf("`use %s`: no xo.mod with a module line in %s", f[1], dir)})
				continue
			}
			probs = append(probs, ps...)
			if prev, ok := seen[mf.Path]; ok {
				probs = append(probs, &Problem{Code: diag.ModFileInvalid, File: name, Line: line, Msg: fmt.Sprintf("module %s is used twice (first on line %d)", mf.Path, prev)})
				continue
			}
			seen[mf.Path] = line
			w.Modules = append(w.Modules, mf)
		default:
			probs = append(probs, &Problem{Code: diag.ModFileInvalid, File: name, Line: line, Msg: "malformed xo.work line: want `xo <edition>` or `use <dir>`"})
		}
	}
	return w, probs
}

// WorkspaceDirs returns dirs plus the directory of every module of the
// workspace that applies to them (check, test, query, and edit treat a
// workspace as one program). dirs is returned unchanged without one.
func WorkspaceDirs(dirs []string) []string {
	if WorkOff() || len(dirs) == 0 {
		return dirs
	}
	root := ModRoot(dirs[0])
	if root == "" {
		return dirs
	}
	w, _ := FindWork(root)
	if w == nil {
		return dirs
	}
	out := append([]string(nil), dirs...)
	have := map[string]bool{}
	for _, d := range dirs {
		if abs, err := filepath.Abs(d); err == nil {
			have[abs] = true
		}
	}
	for _, m := range w.Modules {
		covered := false
		for d := range have {
			if d == m.Dir || strings.HasPrefix(m.Dir, d+string(filepath.Separator)) {
				covered = true
			}
		}
		if !covered {
			out = append(out, relTo(m.Dir))
			have[m.Dir] = true
		}
	}
	return out
}

// relTo makes dir relative to the working directory when it lies below it.
func relTo(dir string) string {
	wd, err := os.Getwd()
	if err != nil {
		return dir
	}
	if rel, err := filepath.Rel(wd, dir); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return dir
}

// ModRoot is the directory of the nearest xo.mod at or above dir, or "".
func ModRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for d := abs; ; {
		if _, err := os.Stat(filepath.Join(d, "xo.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}
