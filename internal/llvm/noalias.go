package llvm

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ssa"
)

// Alias scopes of lists (decision 0211; mirrors selfhost/llvm
// noalias.xo, the source of truth, which documents the proof).
//
// The values of List type in a function fall into classes whose objects
// are never shared within one call: a U class starts at the reuse of a
// parameter or at a new list (list.new) and takes the values updated in
// place from it (list.set, list.push, a further reuse), the `var`
// results of direct calls that hand back the object they were given or
// a new one (a summary per function), and block parameters that join
// only its values; the RO class holds the borrowed List, Str, and Bytes
// parameters. Any other value (a raw parameter before its reuse, a field,
// an element, another call's result) has no class, and a class a block
// parameter joins with such a value, or the RO class joined with a U
// class, has none either. Each class with an access gets a scope (all of
// RO one scope); a list access carries !alias.scope of its class and
// !noalias of the others, the entry declares the scopes, and writes are
// scoped only through a U class. Fewer than two scopes: nothing.

const (
	// scopeBase is the first metadata id of function 0: function fi's
	// nodes are scopeBase*(fi+1) and up (ids may skip)
	scopeBase = 100
	// scopeMax is the most scopes a function gets: a class past it keeps
	// no scope
	scopeMax = 32
)

// The facts of a class (classFlags).
const (
	clPoison = 1 << iota // a value of no class joins it
	clRO                 // a borrowed parameter
	clW                  // a value an update, reuse, or call made
	clRoot               // the reuse of a parameter, or a new list
)

// classes is the union-find of the List, Str, and Bytes values of one
// function.
type classes struct {
	parent map[*ssa.Value]*ssa.Value
	flags  map[*ssa.Value]int
	// root is, per class root, the parameter whose reuse it holds: its
	// index, -1 for none, -2 for several
	root map[*ssa.Value]int
}

func (c *classes) find(v *ssa.Value) *ssa.Value {
	p, ok := c.parent[v]
	if !ok {
		// a value no block defines: no class
		c.parent[v] = v
		c.flags[v] = clPoison
		c.root[v] = -1
		return v
	}
	if p == v {
		return v
	}
	r := c.find(p)
	c.parent[v] = r
	return r
}

func (c *classes) union(a, b *ssa.Value) {
	ra, rb := c.find(a), c.find(b)
	if ra == rb {
		return
	}
	// the root with the smaller id stays: the classes do not depend on
	// the order of the unions
	if rb.ID < ra.ID {
		ra, rb = rb, ra
	}
	c.parent[rb] = ra
	c.flags[ra] |= c.flags[rb]
	switch x, y := c.root[ra], c.root[rb]; {
	case x == -1:
		c.root[ra] = y
	case y == -1 || x == y:
	default:
		c.root[ra] = -2
	}
}

func (c *classes) def(v *ssa.Value, fl, root int) {
	c.parent[v] = v
	c.flags[v] = fl
	c.root[v] = root
}

// scoped reports whether v has a List, Str, or Bytes type.
func scoped(v *ssa.Value) bool {
	return v != nil && v.Type != nil && (v.Type.Kind == ssa.KList || v.Type.Kind == ssa.KStr)
}

// isUpdate reports whether v is a list.set or list.push: the list it
// returns is its first argument's, updated in place.
func isUpdate(v *ssa.Value) bool {
	return v.Op == ssa.OpCall && v.Type.Kind == ssa.KList && (v.Aux == "list.set" || v.Aux == "list.push")
}

// isNewList reports whether v is list.new or rt.rl_empty (the zero list
// of an unwind exit): a list allocated here, or the
// shared empty list, which no operation writes.
func isNewList(v *ssa.Value) bool {
	return v.Op == ssa.OpCall && v.Type.Kind == ssa.KList && (v.Aux == "list.new" || v.Aux == "rt.rl_empty")
}

// handedBack returns, for a `var` result of a direct call (the Field k
// >= 1 of the call), the argument it hands back when the callee's
// summary says the result is that argument's object or a new one; nil
// otherwise.
func (g *sgen) handedBack(v *ssa.Value) *ssa.Value {
	if v.Op != ssa.OpField || v.Type.Kind != ssa.KList || v.Args[0].Op != ssa.OpCall {
		return nil
	}
	c := v.Args[0]
	name, _ := c.Aux.(string)
	callee := g.byName[name]
	if callee == nil {
		return nil
	}
	k, _ := v.Aux.(int)
	j := k - 1
	same := g.sameVar[callee]
	if j < 0 || j >= len(same) || !same[j] {
		return nil
	}
	i := callee.VarParams[j]
	if i >= len(c.Args) {
		return nil
	}
	return c.Args[i]
}

// classesOf groups the values of f (classes above).
func (g *sgen) classesOf(f *ssa.Func) *classes {
	c := &classes{parent: map[*ssa.Value]*ssa.Value{}, flags: map[*ssa.Value]int{}, root: map[*ssa.Value]int{}}
	byRef := g.byRefOf(f)
	params := map[*ssa.Value]int{}
	for i, p := range f.Params() {
		params[p] = i
	}
	// the definitions
	for bi, b := range f.Blocks {
		for _, p := range b.Params {
			if !scoped(p) {
				continue
			}
			if bi == 0 {
				i := params[p]
				if i < len(f.Borrowed) && f.Borrowed[i] && !isVarParam(f, i) && (byRef == nil || !byRef[i]) {
					c.def(p, clRO, -1)
				} else {
					c.def(p, clPoison, -1)
				}
				continue
			}
			c.def(p, 0, -1)
		}
		for _, v := range b.Instrs {
			if !scoped(v) {
				continue
			}
			switch {
			case v.Op == ssa.OpReuse:
				if i, ok := params[v.Args[0]]; ok {
					c.def(v, clW|clRoot, i)
				} else {
					c.def(v, clW, -1)
				}
			case isUpdate(v), g.handedBack(v) != nil:
				c.def(v, clW, -1)
			case isNewList(v):
				c.def(v, clW|clRoot, -1)
			default:
				c.def(v, clPoison, -1)
			}
		}
	}
	// the joins
	reuseOf := map[*ssa.Value]*ssa.Value{}
	for bi, b := range f.Blocks {
		if bi > 0 {
			for k, p := range b.Params {
				if !scoped(p) {
					continue
				}
				for _, e := range b.Preds {
					if k < len(e.Args) && e.Args[k] != nil {
						c.union(p, e.Args[k])
					} else {
						c.flags[c.find(p)] |= clPoison
					}
				}
			}
		}
		for _, v := range b.Instrs {
			if !scoped(v) {
				continue
			}
			switch {
			case v.Op == ssa.OpReuse:
				a := v.Args[0]
				if _, ok := params[a]; ok {
					// the reuses of one parameter are one class
					if r, ok := reuseOf[a]; ok {
						c.union(v, r)
					} else {
						reuseOf[a] = v
					}
				} else {
					c.union(v, a)
				}
			case isUpdate(v):
				c.union(v, v.Args[0])
			default:
				if a := g.handedBack(v); a != nil {
					c.union(v, a)
				}
			}
		}
	}
	return c
}

// sameVarOf is the summary of f: per `var` parameter, whether every
// return hands back its own object or a new list: the parameter itself,
// a list.new, or a value of the class of its reuse that holds no other
// parameter's reuse and has a class (U).
func (g *sgen) sameVarOf(f *ssa.Func) []bool {
	c := g.classesOf(f)
	ps := f.Params()
	out := make([]bool, len(f.VarParams))
	for j, i := range f.VarParams {
		p := ps[i]
		ok := p.Type.Kind == ssa.KList
		for _, b := range f.Blocks {
			t := b.Term()
			if !ok || t == nil || t.Op != ssa.OpReturn {
				continue
			}
			if 1+j >= len(t.Args) {
				ok = false
				break
			}
			r := t.Args[1+j]
			if r == p || isNewList(r) {
				continue
			}
			if !scoped(r) {
				ok = false
				break
			}
			k := c.find(r)
			if c.flags[k]&(clPoison|clRO) != 0 || c.root[k] != i && c.root[k] != -1 {
				ok = false
			}
		}
		out[j] = ok
	}
	return out
}

// summarize computes sameVar for every function: from none, until no
// summary changes (each only turns true, so it ends).
func (g *sgen) summarize() {
	g.sameVar = map[*ssa.Func][]bool{}
	for {
		changed := false
		for _, f := range g.m.Funcs {
			s := g.sameVarOf(f)
			old := g.sameVar[f]
			for j := range s {
				if j >= len(old) || old[j] != s[j] {
					changed = true
					break
				}
			}
			g.sameVar[f] = s
		}
		if !changed {
			return
		}
	}
}

// scopeInfo is the alias scopes of one function.
type scopeInfo struct {
	tag  map[*ssa.Value]int // an access's scope (list operations, Bytes reads)
	n    int
	base int
}

// md is the metadata suffix of access v: its scope and the others, or "".
func (sc *scopeInfo) md(v *ssa.Value) string {
	if sc == nil {
		return ""
	}
	k, ok := sc.tag[v]
	if !ok {
		return ""
	}
	return fmt.Sprintf(", !alias.scope !%d, !noalias !%d", sc.base+1+scopeMax+k, sc.base+1+2*scopeMax+k)
}

// decls are the scope declarations at the entry.
func (sc *scopeInfo) decls() string {
	if sc == nil {
		return ""
	}
	var b strings.Builder
	for k := 0; k < sc.n; k++ {
		fmt.Fprintf(&b, "  call void @llvm.experimental.noalias.scope.decl(metadata !%d)\n", sc.base+1+scopeMax+k)
	}
	return b.String()
}

// nodes are the metadata of the scopes: the domain, each scope, its
// list, and the list of the others.
func (sc *scopeInfo) nodes() string {
	if sc == nil {
		return ""
	}
	var b strings.Builder
	d := sc.base
	fmt.Fprintf(&b, "!%d = distinct !{!%d}\n", d, d)
	for k := 0; k < sc.n; k++ {
		s := d + 1 + k
		fmt.Fprintf(&b, "!%d = distinct !{!%d, !%d}\n", s, s, d)
	}
	for k := 0; k < sc.n; k++ {
		fmt.Fprintf(&b, "!%d = !{!%d}\n", d+1+scopeMax+k, d+1+k)
	}
	for k := 0; k < sc.n; k++ {
		var others []string
		for j := 0; j < sc.n; j++ {
			if j != k {
				others = append(others, fmt.Sprintf("!%d", d+1+j))
			}
		}
		fmt.Fprintf(&b, "!%d = !{%s}\n", d+1+2*scopeMax+k, strings.Join(others, ", "))
	}
	return b.String()
}

// scopeRead names the operations that read their list or Bytes operand
// (Args[0]); scopeWrite those that also write the list.
var (
	scopeRead  = map[string]bool{"list.len": true, "list.index": true, "list.get": true, "bytes.index": true, "rt.bytes_word": true, "rt.bytes_eq16": true, "rt.bytes_high16": true}
	scopeWrite = map[string]bool{"list.set": true, "list.push": true, "list.pop": true, "list.take": true}
)

// scopesOf gives the accesses of function f (index fi) their scopes, or
// nil when it has fewer than two.
func (g *sgen) scopesOf(f *ssa.Func, fi int) *scopeInfo {
	c := g.classesOf(f)
	sc := &scopeInfo{tag: map[*ssa.Value]int{}, base: scopeBase * (fi + 1)}
	scope := map[*ssa.Value]int{}
	ro := -1
	for _, b := range f.Blocks {
		for _, v := range b.Instrs {
			if v.Op != ssa.OpCall || len(v.Args) == 0 {
				continue
			}
			name, _ := v.Aux.(string)
			w := scopeWrite[name]
			if !w && !scopeRead[name] {
				continue
			}
			a := v.Args[0]
			if !scoped(a) {
				continue
			}
			r := c.find(a)
			fl := c.flags[r]
			var k int
			switch {
			case fl&clPoison != 0:
				continue
			case fl&clRO != 0:
				if w || fl&clW != 0 {
					continue
				}
				if ro < 0 {
					if sc.n == scopeMax {
						continue
					}
					ro = sc.n
					sc.n++
				}
				k = ro
			case fl&clRoot != 0:
				kk, ok := scope[r]
				if !ok {
					if sc.n == scopeMax {
						continue
					}
					kk = sc.n
					scope[r] = kk
					sc.n++
				}
				k = kk
			default:
				continue
			}
			sc.tag[v] = k
		}
	}
	if sc.n < 2 {
		return nil
	}
	return sc
}
