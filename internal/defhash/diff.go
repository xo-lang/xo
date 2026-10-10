package defhash

import (
	"sort"
	"strings"
)

// Change is a definition that differs between two versions of a module.
type Change struct {
	// Kind is added, removed, changed (the definition itself changed),
	// or changed_via (only definitions it references changed).
	Kind     string `json:"kind"`
	Package  string `json:"package"`
	Name     string `json:"name"`
	ItemKind string `json:"item_kind"`
	Pub      bool   `json:"pub"`
	// Via are the referenced definitions whose hashes changed (for
	// changed_via, and for changed when some did too).
	Via  []string `json:"via,omitempty"`
	File string   `json:"file,omitempty"` // the new definition
	Line int      `json:"line,omitempty"`
}

// Diff compares the definitions of two versions of a module, matched by
// package directory and name, so a new major version path compares with
// the old one. Unchanged definitions are left out.
func Diff(old, new []Def) []Change {
	oldBy := map[string]Def{}
	for _, d := range old {
		oldBy[d.key] = d
	}
	newBy := map[string]Def{}
	for _, d := range new {
		newBy[d.key] = d
	}
	var out []Change
	for _, d := range new {
		o, ok := oldBy[d.key]
		ch := Change{Package: d.Package, Name: d.Name, ItemKind: d.Kind, Pub: d.Pub, File: d.File, Line: d.Line}
		switch {
		case !ok:
			ch.Kind = "added"
		case o.Hash == d.Hash && o.Kind == d.Kind:
			continue
		default:
			for _, r := range d.refs {
				ro, ok1 := oldBy[r]
				rn, ok2 := newBy[r]
				if !ok1 || !ok2 || ro.Hash != rn.Hash {
					ch.Via = append(ch.Via, rn.Package+"."+rn.Name)
				}
			}
			ch.Kind = "changed"
			if o.Own == d.Own && o.Kind == d.Kind {
				ch.Kind = "changed_via"
			}
		}
		out = append(out, ch)
	}
	for _, o := range old {
		if _, ok := newBy[o.key]; !ok {
			out = append(out, Change{Kind: "removed", Package: o.Package, Name: o.Name, ItemKind: o.Kind, Pub: o.Pub})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Package != out[j].Package {
			return out[i].Package < out[j].Package
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Describe is a one line description of a change for text output.
func (c Change) Describe() string {
	name := c.Package + "." + c.Name
	vis := ""
	if !c.Pub {
		vis = "private "
	}
	switch c.Kind {
	case "added":
		return "added " + vis + c.ItemKind + " " + name
	case "removed":
		return "removed " + vis + c.ItemKind + " " + name
	case "changed_via":
		return vis + c.ItemKind + " " + name + " changed through " + strings.Join(c.Via, ", ")
	}
	s := "changed " + vis + c.ItemKind + " " + name
	if len(c.Via) > 0 {
		s += " (and through " + strings.Join(c.Via, ", ") + ")"
	}
	return s
}

// Find returns the definitions name refers to: an ID
// (`example.com/app/users.find`), a name (`find`, `Point.norm`), or a
// name qualified by the last element of its package (`users.find`).
func Find(defs []Def, name string) []Def {
	var out []Def
	for _, d := range defs {
		last := d.Package
		if i := strings.LastIndexByte(last, '/'); i >= 0 {
			last = last[i+1:]
		}
		if d.ID == name || d.Name == name || last+"."+d.Name == name {
			out = append(out, d)
		}
	}
	return out
}

// Nearest lists up to n definition names closest to name (edit
// distance), for a name that resolves to none.
func Nearest(defs []Def, name string, n int) []string {
	type cand struct {
		name string
		d    int
	}
	var cs []cand
	seen := map[string]bool{}
	for _, d := range defs {
		if seen[d.Name] {
			continue
		}
		seen[d.Name] = true
		cs = append(cs, cand{d.Name, distance(strings.ToLower(name), strings.ToLower(d.Name))})
	}
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].d != cs[j].d {
			return cs[i].d < cs[j].d
		}
		return cs[i].name < cs[j].name
	})
	var out []string
	for _, c := range cs {
		if len(out) == n || c.d > max(3, len(name)/2) {
			break
		}
		out = append(out, c.name)
	}
	return out
}

func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
