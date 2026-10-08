package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Builtin effect labels and their sub effects (core 6.1).
var effectLabels = map[string]bool{
	"net": true, "net.dial": true, "net.listen": true,
	"fs": true, "fs.read": true, "fs.write": true,
	"clock": true, "rand": true, "env": true, "proc": true, "stdio": true, "ffi": true,
}

// EffVar is an effect inference variable, created when a generic with an
// `fx: Effects` parameter is instantiated.
type EffVar struct {
	Ref *EffSet
}

// EffItem is one member of an effect set. Exactly one of Label, Param,
// EVar, Fn is set. Span and Via record where the effect is performed, for
// the call chain in XO0701 notes.
type EffItem struct {
	Label string
	Param *TParam
	EVar  *EffVar
	Fn    *FuncObj // the (inferred) effects of a non pub function
	// Sub instantiates the effect parameters of Fn at this call.
	Sub  *substMap
	Span token.Span
	Via  string
}

// EffSet is a set of effects: labels, effect parameters, open effect
// variables, and references to the inferred effects of functions.
type EffSet struct {
	Items []EffItem
}

func (s *EffSet) add(it EffItem) {
	s.Items = append(s.Items, it)
}

func (s *EffSet) addAll(o *EffSet) {
	if o == nil {
		return
	}
	s.Items = append(s.Items, o.Items...)
}

// performed returns a copy of s in which declaration items (no span) are
// attributed to the call at sp of the callee via.
func (s *EffSet) performed(sp token.Span, via string) *EffSet {
	if s == nil {
		return nil
	}
	out := &EffSet{}
	for _, it := range s.Items {
		if it.Span.File == "" {
			it.Span = sp
			it.Via = via
		}
		out.Items = append(out.Items, it)
	}
	return out
}

func (s *EffSet) subst(m *substMap) *EffSet {
	return s.substDepth(m, 0)
}

func (s *EffSet) substDepth(m *substMap, depth int) *EffSet {
	if s == nil || m.empty() || depth > 50 {
		return s
	}
	out := &EffSet{}
	for _, it := range s.Items {
		if it.EVar != nil && it.EVar.Ref != nil {
			// a bound inference variable (an elided effect argument of a
			// result type): substitute what it stands for
			out.Items = append(out.Items, it.EVar.Ref.substDepth(m, depth+1).Items...)
			continue
		}
		if it.Fn != nil && it.Sub == nil {
			it.Sub = m
		}
		if it.Param != nil {
			if r, ok := m.effs[it.Param]; ok {
				for _, ri := range r.Items {
					if ri.Span.File == "" {
						ri.Span, ri.Via = it.Span, it.Via
					}
					out.Items = append(out.Items, ri)
				}
				continue
			}
		}
		out.Items = append(out.Items, it)
	}
	return out
}

// openVar returns the first unbound effect variable in s.
func (s *EffSet) openVar() *EffVar {
	if s == nil {
		return nil
	}
	for _, it := range s.Items {
		if it.EVar != nil {
			if it.EVar.Ref == nil {
				return it.EVar
			}
			if v := it.EVar.Ref.openVar(); v != nil {
				return v
			}
		}
	}
	return nil
}

func (s *EffSet) String() string {
	if s == nil {
		return ""
	}
	f := flatten(s, nil)
	return f.String()
}

// flatEff is an effect set with function references expanded: labels and
// effect parameters, each with the item that witnesses it.
type flatEff struct {
	labels map[string]EffItem
	params map[*TParam]EffItem
}

func newFlat() *flatEff {
	return &flatEff{labels: map[string]EffItem{}, params: map[*TParam]EffItem{}}
}

func (f *flatEff) size() int { return len(f.labels) + len(f.params) }

func (f *flatEff) String() string {
	var parts []string
	for l := range f.labels {
		parts = append(parts, l)
	}
	for p := range f.params {
		parts = append(parts, p.Name)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// covers reports whether label l is in f directly or through its parent
// (`fs.read` is covered by `fs`).
func (f *flatEff) coversLabel(l string) bool {
	if _, ok := f.labels[l]; ok {
		return true
	}
	if i := strings.IndexByte(l, '.'); i > 0 {
		_, ok := f.labels[l[:i]]
		return ok
	}
	return false
}

// flatten expands s. inferred holds the current effects of inferred
// functions (nil while they are not known: function items are dropped).
func flatten(s *EffSet, inferred map[*FuncObj]*flatEff) *flatEff {
	out := newFlat()
	flattenInto(out, s, inferred, 0)
	return out
}

func flattenInto(out *flatEff, s *EffSet, inferred map[*FuncObj]*flatEff, depth int) {
	if s == nil || depth > 50 {
		return
	}
	for _, it := range s.Items {
		switch {
		case it.Label != "":
			if _, ok := out.labels[it.Label]; !ok {
				out.labels[it.Label] = it
			}
		case it.Param != nil:
			if _, ok := out.params[it.Param]; !ok {
				out.params[it.Param] = it
			}
		case it.EVar != nil:
			flattenInto(out, it.EVar.Ref, inferred, depth+1)
		case it.Fn != nil:
			if it.Fn.Declared != nil {
				flattenInto(out, it.Fn.Declared.performed(it.Span, it.Fn.DisplayName()), inferred, depth+1)
				continue
			}
			if inferred == nil {
				continue
			}
			for l := range inferred[it.Fn].labels {
				if _, ok := out.labels[l]; !ok {
					w := it
					out.labels[l] = w
				}
			}
			for p := range inferred[it.Fn].params {
				if it.Sub != nil {
					if r, ok := it.Sub.effs[p]; ok {
						flattenInto(out, r.performed(it.Span, it.Via), inferred, depth+1)
						continue
					}
				}
				if _, ok := out.params[p]; !ok {
					out.params[p] = it
				}
			}
		}
	}
}

// effObligation requires the effects of sub to be within sup.
type effObligation struct {
	sub  *EffSet
	sup  *EffSet
	span token.Span
	// what describes the requirement for the message, e.g.
	// "`fetch` declares `uses clock`".
	what func(sup string) string
	pure bool // contract clauses: must be pure regardless of sup
}

// solveEffects computes the effects of every inferred function by fixpoint
// and then checks every obligation (XO0701).
func (c *checker) solveEffects() {
	inferred := map[*FuncObj]*flatEff{}
	for _, f := range c.inferredFns {
		inferred[f] = newFlat()
	}
	for changed := true; changed; {
		changed = false
		for _, f := range c.inferredFns {
			nf := flatten(f.body, inferred)
			if nf.size() != inferred[f].size() {
				inferred[f] = nf
				changed = true
			}
		}
	}
	c.inferred = inferred
	seen := map[string]bool{}
	for _, ob := range c.obligations {
		sub := flatten(ob.sub, inferred)
		sup := flatten(ob.sup, inferred)
		var missing []string
		var first EffItem
		labels := make([]string, 0, len(sub.labels))
		for l := range sub.labels {
			labels = append(labels, l)
		}
		sort.Strings(labels)
		for _, l := range labels {
			if !sup.coversLabel(l) {
				if len(missing) == 0 {
					first = sub.labels[l]
				}
				missing = append(missing, l)
			}
		}
		var params []string
		for p, it := range sub.params {
			if _, ok := sup.params[p]; !ok {
				if len(missing) == 0 && len(params) == 0 {
					first = it
				}
				params = append(params, p.Name)
			}
		}
		sort.Strings(params)
		missing = append(missing, params...)
		if len(missing) == 0 {
			continue
		}
		key := fmt.Sprintf("%v|%v", ob.span, missing)
		if seen[key] {
			continue
		}
		seen[key] = true
		list := "`" + strings.Join(missing, "`, `") + "`"
		var msg string
		if ob.pure {
			msg = fmt.Sprintf("contract clauses must be pure, but this one performs %s", list)
		} else {
			msg = fmt.Sprintf("%s, but performs %s", ob.what(sup.String()), list)
		}
		d := c.errorf(diag.UndeclaredEffect, ob.span, "%s", msg)
		if d == nil {
			continue
		}
		key0 := ""
		if len(missing) > 0 {
			key0 = missing[0]
		}
		c.effectChain(d, first, key0, inferred)
	}
}

// effectChain appends the call chain that performs effect key, starting at
// item w, as notes on d.
func (c *checker) effectChain(d *diag.Diagnostic, w EffItem, key string, inferred map[*FuncObj]*flatEff) {
	visited := map[*FuncObj]bool{}
	for i := 0; i < 20; i++ {
		sp := diag.FromSpan(w.Span)
		if w.Fn != nil && w.Fn.Declared == nil {
			d.Notes = append(d.Notes, diag.NoteItem{Message: fmt.Sprintf("calls `%s`, which performs `%s`", w.Fn.DisplayName(), key), Span: &sp})
			if visited[w.Fn] {
				return
			}
			visited[w.Fn] = true
			fl := inferred[w.Fn]
			if fl == nil {
				return
			}
			next, ok := fl.labels[key]
			if !ok {
				for p, it := range fl.params {
					if p.Name == key {
						next, ok = it, true
					}
				}
			}
			if !ok {
				return
			}
			w = next
			continue
		}
		via := w.Via
		if w.Fn != nil {
			via = w.Fn.DisplayName()
		}
		if via == "" {
			via = "this"
		}
		if w.Span.File != "" {
			d.Notes = append(d.Notes, diag.NoteItem{Message: fmt.Sprintf("`%s` uses `%s` here", via, key), Span: &sp})
		}
		return
	}
}
