package mod

import (
	"fmt"
	"strings"
)

// Ceiling is the effect ceiling of a dependency (decision 0086 step 3):
// `require <path> <version> uses none` or `uses net, fs.read` in xo.mod,
// or `use <dir> uses ...` in xo.work. Every call from the requiring
// module into the dependency must perform only effects within it. An
// effect is within it when it or its parent label is listed (`fs` admits
// `fs.read`); `uses none` admits nothing.
type Ceiling struct {
	Effects []string
}

// EffectLabels are the effect labels a ceiling may name (core 6.1, and
// `go` from decision 0041). Kept equal to the checker's by a test.
var EffectLabels = []string{"net", "net.dial", "net.listen", "fs", "fs.read", "fs.write", "clock", "rand", "env", "proc", "stdio", "ffi", "go"}

// Admits reports whether effect label l is within the ceiling. A nil
// ceiling admits everything.
func (c *Ceiling) Admits(l string) bool {
	if c == nil {
		return true
	}
	for _, e := range c.Effects {
		if e == l || strings.HasPrefix(l, e+".") {
			return true
		}
	}
	return false
}

// String is the clause as written: `uses none` or `uses net, fs.read`.
func (c *Ceiling) String() string {
	if c == nil {
		return ""
	}
	if len(c.Effects) == 0 {
		return "uses none"
	}
	return "uses " + strings.Join(c.Effects, ", ")
}

// ParseCeiling parses the words after a require's version (or a `use`
// directory): `uses none`, `uses net, fs.read`, or `uses (net, fs.read)`.
func ParseCeiling(words []string) (*Ceiling, error) {
	if len(words) == 0 || words[0] != "uses" {
		return nil, fmt.Errorf("want `uses <effects>` or `uses none`")
	}
	rest := strings.TrimSpace(strings.Join(words[1:], " "))
	if strings.HasPrefix(rest, "(") && strings.HasSuffix(rest, ")") {
		rest = strings.TrimSpace(rest[1 : len(rest)-1])
	}
	if rest == "" {
		return nil, fmt.Errorf("`uses` names no effects; write `uses none` for a dependency that may perform none")
	}
	c := &Ceiling{Effects: []string{}}
	if rest == "none" {
		return c, nil
	}
	seen := map[string]bool{}
	for _, e := range strings.Split(rest, ",") {
		e = strings.TrimSpace(e)
		known := false
		for _, l := range EffectLabels {
			if e == l {
				known = true
			}
		}
		if !known {
			return nil, fmt.Errorf("unknown effect %q (want none, or labels among %s)", e, strings.Join(EffectLabels, ", "))
		}
		if !seen[e] {
			seen[e] = true
			c.Effects = append(c.Effects, e)
		}
	}
	return c, nil
}

// Ceilings returns the effect ceilings that apply to calls from the
// module in dir, by dependency module path: those of its xo.mod requires
// and, when a workspace uses the module, those of the workspace's `use`
// lines for the other modules. Each entry names where it is written.
func Ceilings(dir string) map[string][]CeilingSource {
	out := map[string][]CeilingSource{}
	f, _ := ReadFile(dir)
	if f == nil {
		return out
	}
	for _, r := range f.Requires {
		if r.Ceiling != nil {
			out[r.Path] = append(out[r.Path], CeilingSource{Ceiling: r.Ceiling, File: f.File, Line: r.Line, Version: r.Version})
		}
	}
	if w, _ := FindWork(dir); w != nil {
		for i, m := range w.Modules {
			if m.Path == f.Path || w.Ceilings[i] == nil {
				continue
			}
			out[m.Path] = append(out[m.Path], CeilingSource{Ceiling: w.Ceilings[i], File: w.File, Line: w.Lines[i]})
		}
	}
	return out
}

// CeilingSource is a ceiling with the xo.mod or xo.work line declaring it.
type CeilingSource struct {
	Ceiling *Ceiling
	File    string
	Line    int
	Version string // the required version ("" for an xo.work line)
}
