package mod

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
)

// Vouch is one `vouch` line of an xo.mod (decision 0156): the consuming
// module's review of an escape hatch of one dependency version, a `use c`
// header or a `use go` package.
//
//	vouch github.com/acme/zip v1.2.0 c "zlib.h" uses none
//	vouch github.com/acme/pg v1.0.0 go "github.com/jackc/pgx/v5"
type Vouch struct {
	Path    string // module path of the dependency
	Version string // the exact version reviewed
	Kind    string // "c" or "go"
	Target  string // the header (c) or the Go import path (go), as the `use` line writes it
	// Effects is the `uses` of a c vouch: the effects the consumer
	// accepts for the binding; nil without `uses` (the binding performs
	// `ffi`). Always nil for go.
	Effects *Ceiling
	File    string
	Line    int
}

// String is the line as written.
func (v Vouch) String() string {
	s := fmt.Sprintf("vouch %s %s %s %q", v.Path, v.Version, v.Kind, v.Target)
	if v.Effects != nil {
		s += " " + v.Effects.String()
	}
	return s
}

const vouchSyntax = "want `vouch <module path> <version> c \"<header>\" [uses <effects|none>]` or `vouch <module path> <version> go \"<Go import path>\"` (decision 0156)"

func parseVouch(name string, line int, fields []string) (Vouch, *Problem) {
	bad := func(msg string) (Vouch, *Problem) {
		return Vouch{}, &Problem{Code: diag.ModFileInvalid, File: name, Line: line, Msg: "malformed vouch: " + msg, Notes: []string{vouchSyntax}}
	}
	if len(fields) < 5 {
		return bad("too few fields")
	}
	v := Vouch{Path: fields[1], Version: fields[2], Kind: fields[3], File: name, Line: line}
	if !ValidPath(v.Path) {
		return bad(fmt.Sprintf("invalid module path %q", v.Path))
	}
	if !ValidVersion(v.Version) {
		return bad(fmt.Sprintf("invalid version %q: a vouch names the exact vX.Y.Z it reviewed", v.Version))
	}
	if v.Kind != "c" && v.Kind != "go" {
		return bad(fmt.Sprintf("unknown kind %q: want c or go", v.Kind))
	}
	t, err := strconv.Unquote(fields[4])
	if err != nil || t == "" {
		return bad(fmt.Sprintf("the header or Go package must be a quoted string, not %s", fields[4]))
	}
	v.Target = t
	if len(fields) > 5 {
		if v.Kind == "go" {
			return bad("a go vouch takes no `uses`: calls into Go always perform `go`")
		}
		c, err := ParseCeiling(fields[5:])
		if err != nil {
			return bad(err.Error())
		}
		for _, e := range c.Effects {
			if e == "go" {
				return bad("a C binding cannot perform `go`")
			}
		}
		v.Effects = c
	}
	return v, nil
}

// VouchesOf reads the vouch lines of the xo.mod files in dirs (the
// consuming modules: the main module and the workspace's modules).
func VouchesOf(dirs []string) []Vouch {
	var out []Vouch
	seen := map[string]bool{}
	for _, d := range dirs {
		if abs, err := filepath.Abs(d); err == nil {
			d = abs
		}
		if seen[d] {
			continue
		}
		seen[d] = true
		if f, _ := ReadFile(d); f != nil {
			out = append(out, f.Vouches...)
		}
	}
	return out
}

// FindVouch returns the vouch for the escape hatch kind/target of module
// path at version, or else the vouches of the same hatch at other
// versions (stale reviews).
func FindVouch(vs []Vouch, path, version, kind, target string) (*Vouch, []Vouch) {
	var stale []Vouch
	for i := range vs {
		v := vs[i]
		if v.Path != path || v.Kind != kind || v.Target != target {
			continue
		}
		if v.Version == version {
			return &vs[i], nil
		}
		stale = append(stale, v)
	}
	return nil, stale
}

// SuggestVouch is the vouch line that would cover an escape hatch:
// for C, with the dependency's claimed effects (claim nil: none given).
func SuggestVouch(path, version, kind, target string, claim []string) string {
	s := fmt.Sprintf("vouch %s %s %s %q", path, version, kind, target)
	if kind == "c" && claim != nil {
		if len(claim) == 0 {
			s += " uses none"
		} else {
			s += " uses " + strings.Join(claim, ", ")
		}
	}
	return s
}
