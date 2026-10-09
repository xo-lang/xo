// Package mod implements Xo modules and dependencies (core 4.2, 4.3;
// decision 0086): `require` lines in xo.mod, minimal version selection
// over the transitive requirement graph, fetching module versions from
// git into a read only module cache, `xo.lock` content hashes verified on
// every build, vendoring, offline builds, and `xo.work` workspaces.
//
// No code runs at fetch time: a module version is the tree of its git tag,
// extracted with `git archive`; nothing in it is executed.
package mod

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
)

// Require is one `require <module path> <version> [uses <effects|none>]`
// line of an xo.mod.
type Require struct {
	Path    string
	Version string
	Line    int // 1 based line in xo.mod
	// Ceiling is the effect ceiling of the dependency; nil without a
	// `uses` clause (no ceiling).
	Ceiling *Ceiling
}

// File is the module part of a parsed xo.mod: its path and the Xo
// requirements. `require go` lines (Go modules for `use go`, core 4.4),
// `edition`, and `gc` lines are read elsewhere.
type File struct {
	Path     string // the `module` line
	Requires []Require
	File     string // path of the xo.mod file
	Dir      string // directory holding it
}

// Problem is a module error: a diagnostic about xo.mod, xo.lock, xo.work,
// the module cache, or fetching.
type Problem struct {
	Code diag.Code
	// File and Line locate the problem (an xo.mod require line, a lock
	// line); Line 0 means the start of File.
	File string
	Line int
	Msg  string
	// Notes are extra lines (how to fix it).
	Notes []string
	// Module is the module path the problem is about, if any: imports
	// of its packages are not reported again as unresolved.
	Module string
}

func (p *Problem) Error() string {
	loc := p.File
	if p.Line > 0 {
		loc += ":" + strconv.Itoa(p.Line)
	}
	s := string(p.Code) + ": " + p.Msg
	if loc != "" {
		s = loc + ": " + s
	}
	for _, n := range p.Notes {
		s += "\n  note: " + n
	}
	return s
}

var versionRE = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)

// ValidVersion reports whether v is a semantic version tag vX.Y.Z (with
// an optional -prerelease).
func ValidVersion(v string) bool { return versionRE.MatchString(v) }

// Compare orders two valid versions: -1, 0, or 1. A prerelease sorts
// before its release; prereleases compare by their dot separated parts.
func Compare(a, b string) int {
	ma, mb := versionRE.FindStringSubmatch(a), versionRE.FindStringSubmatch(b)
	if ma == nil || mb == nil {
		return strings.Compare(a, b)
	}
	for i := 1; i <= 3; i++ {
		if c := cmpNum(ma[i], mb[i]); c != 0 {
			return c
		}
	}
	pa, pb := strings.TrimPrefix(ma[4], "-"), strings.TrimPrefix(mb[4], "-")
	switch {
	case pa == pb:
		return 0
	case pa == "":
		return 1
	case pb == "":
		return -1
	}
	xa, xb := strings.Split(pa, "."), strings.Split(pb, ".")
	for i := 0; i < len(xa) && i < len(xb); i++ {
		na, ea := strconv.Atoi(xa[i])
		nb, eb := strconv.Atoi(xb[i])
		switch {
		case ea == nil && eb == nil:
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
		case ea == nil:
			return -1
		case eb == nil:
			return 1
		default:
			if c := strings.Compare(xa[i], xb[i]); c != 0 {
				return c
			}
		}
	}
	switch {
	case len(xa) < len(xb):
		return -1
	case len(xa) > len(xb):
		return 1
	}
	return 0
}

func cmpNum(a, b string) int {
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

// ValidPath reports whether p can be a module path: slash separated
// elements of letters, digits, `.`, `-`, `_`, and `~`, the first one
// holding a dot (a host name), no element `.` or `..`.
func ValidPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return false
	}
	elems := strings.Split(p, "/")
	if !strings.Contains(elems[0], ".") {
		return false
	}
	for _, e := range elems {
		if e == "" || e == "." || e == ".." || strings.HasPrefix(e, ".") {
			return false
		}
		for _, r := range e {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(".-_~", r)) {
				return false
			}
		}
	}
	return true
}

// IsRemote reports whether an import path names a module that would be
// fetched (its first element holds a dot), as opposed to std/ or a path
// inside the main module.
func IsRemote(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return strings.Contains(first, ".")
}

// ReadFile reads and parses the xo.mod in dir.
func ReadFile(dir string) (*File, []*Problem) {
	name := filepath.Join(dir, "xo.mod")
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, []*Problem{{Code: diag.ModFileInvalid, File: name, Msg: fmt.Sprintf("cannot read xo.mod: %v", err)}}
	}
	return Parse(name, data)
}

// Parse parses the module path and the Xo `require` lines of an xo.mod.
// Malformed require lines are XO1301 problems.
func Parse(name string, data []byte) (*File, []*Problem) {
	f := &File{File: name, Dir: filepath.Dir(name)}
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
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "module":
			if len(fields) == 2 && f.Path == "" {
				f.Path = fields[1]
			}
		case "require":
			if len(fields) >= 2 && fields[1] == "go" {
				continue // `require go <module> <version>` (core 4.4)
			}
			if len(fields) < 3 {
				probs = append(probs, &Problem{Code: diag.ModFileInvalid, File: name, Line: line, Msg: "malformed require: want `require <module path> <version> [uses <effects|none>]`"})
				continue
			}
			p, v := fields[1], fields[2]
			var ceiling *Ceiling
			if len(fields) > 3 {
				c, err := ParseCeiling(fields[3:])
				if err != nil {
					probs = append(probs, &Problem{Code: diag.ModFileInvalid, Module: p, File: name, Line: line, Msg: fmt.Sprintf("malformed effect ceiling for %s: %v", p, err), Notes: []string{"an effect ceiling is `uses none` or `uses net, fs.read` after the version (decision 0086)"}})
					continue
				}
				ceiling = c
			}
			if !ValidPath(p) {
				probs = append(probs, &Problem{Code: diag.ModFileInvalid, File: name, Line: line, Msg: fmt.Sprintf("invalid module path %q in require", p), Notes: []string{"a module path is its location, such as github.com/acme/billing (core 4.2)"}})
				continue
			}
			if !ValidVersion(v) {
				probs = append(probs, &Problem{Code: diag.ModFileInvalid, File: name, Line: line, Msg: fmt.Sprintf("invalid version %q for %s: want a tag vX.Y.Z", v, p)})
				continue
			}
			if msg := MajorMismatch(p, v); msg != "" {
				probs = append(probs, &Problem{Code: diag.ModFileInvalid, Module: p, File: name, Line: line, Msg: msg, Notes: []string{"major versions 2 and above live at a module path ending in /vN, such as github.com/acme/billing/v2 (core 4.2)"}})
				continue
			}
			if prev, ok := seen[p]; ok {
				probs = append(probs, &Problem{Code: diag.ModFileInvalid, File: name, Line: line, Msg: fmt.Sprintf("%s is required twice (first on line %d)", p, prev)})
				continue
			}
			seen[p] = line
			f.Requires = append(f.Requires, Require{Path: p, Version: v, Line: line, Ceiling: ceiling})
		case "replace", "exclude", "retract":
			probs = append(probs, &Problem{Code: diag.ModFileInvalid, File: name, Line: line, Msg: fmt.Sprintf("xo.mod has no `%s` directive", fields[0]), Notes: []string{"point a dependency at local code with an xo.work workspace (decision 0086)"}})
		}
	}
	if f.Path == "" {
		probs = append(probs, &Problem{Code: diag.ModFileInvalid, File: name, Msg: "xo.mod has no `module` line"})
	}
	return f, probs
}

// SetRequire rewrites the xo.mod text data so that it requires path at
// version: the existing require line is changed in place, or a new one is
// added after the last require (or at the end). version "" removes it.
func SetRequire(data []byte, path, version string) []byte {
	lines := strings.Split(string(data), "\n")
	last := -1
	for i, l := range lines {
		fields := strings.Fields(l)
		if len(fields) >= 1 && fields[0] == "require" {
			last = i
			if len(fields) >= 3 && fields[1] == path {
				if version == "" {
					lines = append(lines[:i], lines[i+1:]...)
				} else {
					// keep an effect ceiling after the version
					text, _, _ := strings.Cut(l, "//")
					rest := strings.Fields(text)
					lines[i] = "require " + path + " " + version
					if len(rest) > 3 {
						lines[i] += " " + strings.Join(rest[3:], " ")
					}
				}
				return []byte(strings.Join(lines, "\n"))
			}
		}
	}
	if version == "" {
		return data
	}
	add := "require " + path + " " + version
	if last >= 0 {
		lines = append(lines[:last+1], append([]string{add}, lines[last+1:]...)...)
		return []byte(strings.Join(lines, "\n"))
	}
	s := strings.TrimRight(string(data), "\n")
	return []byte(s + "\n\n" + add + "\n")
}
