package llvm

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ssa"
)

// Alias scopes of list parameters (decision 0211; mirrored from
// selfhost/llvm noalias.xo, which is the source of truth).
//
// Every list is a pointer to its object, so LLVM cannot tell that the
// stores to one `var List[Int]` parameter do not write the elements
// another one reads, and leaves loops over several lists scalar. Xo's
// value semantics say more: a list is written in place only by the one
// holder of its only counted reference (the `reuse` before every update
// copies a shared list), and a borrowed parameter is backed by a
// reference its caller keeps for the whole call. So, within one call of
// a function, the values grouped into classes below never reach a
// common object:
//
//   - a U class: the `reuse` of a parameter (any List parameter: a
//     `var` one, an owned one, or a borrowed one whose reuse copies) and
//     every value derived from it in place (list.set, list.push, a
//     further reuse, block parameters joining only such values). Its
//     objects are the parameter's own object when the reuse found the
//     count 1, or objects it allocated (copies, grown lists). Two
//     parameters that share one object both hold a reference, so at most
//     one reuse keeps it in place, after the other copied;
//   - the RO class: the borrowed List, Str, and Bytes parameters and
//     block parameters joining only them. Their objects existed before
//     the call and stay referenced by the callers, so no U class reuse
//     finds them unique and no U class allocation is one of them.
//
// A value of any other origin (a field, an element, a call's result, a
// raw `var` parameter before its reuse, a tuple of list.pop) is not in a
// class, and a block parameter that joins one with a class value, or
// joins the RO class with a U class, takes its whole class out: its
// accesses keep no scope, which LLVM treats as possibly aliasing every
// access. Each class with an access gets a scope of its own (RO one
// shared scope); a list access through a class value carries
// !alias.scope of its class and !noalias of every other scope, and the
// entry declares the scopes (llvm.experimental.noalias.scope.decl), so
// LLVM's inliner gives each inlined call scopes of its own. Writes are
// scoped only through a U class. With fewer than two scopes nothing is
// emitted.

const (
	// scopeBase is the first metadata id of the function with index 0:
	// function fi's nodes are scopeBase*(fi+1) and up (ids may skip)
	scopeBase = 100
	// scopeMax is the most scopes a function gets: a class past it keeps
	// no scope
	scopeMax = 32
)

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

// scopeRead names the operations that read a list or Bytes operand
// (Args[0]) in place; scopeWrite those that also write the list.
var (
	scopeRead  = map[string]bool{"list.len": true, "list.index": true, "list.get": true, "bytes.index": true, "rt.bytes_word": true, "rt.bytes_eq16": true, "rt.bytes_high16": true}
	scopeWrite = map[string]bool{"list.set": true, "list.push": true, "list.pop": true, "list.take": true}
)

// isUpdate reports whether v is a list.set or list.push: the list it
// returns is its first argument's, updated in place.
func isUpdate(v *ssa.Value) bool {
	return v.Op == ssa.OpCall && v.Type.Kind == ssa.KList && (v.Aux == "list.set" || v.Aux == "list.push")
}

// computeScopes finds the classes of function f (index fi) and the scope
// of each access, or nil when it has fewer than two scopes.
func computeScopes(f *ssa.Func, fi int, byRef []bool) *scopeInfo {
	tracked := func(v *ssa.Value) bool {
		return v != nil && v.Type != nil && (v.Type.Kind == ssa.KList || v.Type.Kind == ssa.KStr)
	}
	const (
		fPoison = 1 << iota
		fRO     // a borrowed parameter
		fW      // a value an update or reuse made
		fRoot   // the reuse of a parameter
	)
	parent := map[*ssa.Value]*ssa.Value{}
	flags := map[*ssa.Value]int{}
	var find func(v *ssa.Value) *ssa.Value
	find = func(v *ssa.Value) *ssa.Value {
		p, ok := parent[v]
		if !ok {
			// a value no block defines: no class
			parent[v] = v
			flags[v] |= fPoison
			return v
		}
		if p == v {
			return v
		}
		r := find(p)
		parent[v] = r
		return r
	}
	union := func(a, b *ssa.Value) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		// the root with the smaller id stays, so the classes do not
		// depend on the order of the unions
		if rb.ID < ra.ID {
			ra, rb = rb, ra
		}
		parent[rb] = ra
		flags[ra] |= flags[rb]
	}
	def := func(v *ssa.Value, fl int) {
		parent[v] = v
		flags[v] = fl
	}
	params := map[*ssa.Value]int{}
	for i, p := range f.Params() {
		params[p] = i
	}
	// the definitions
	for bi, b := range f.Blocks {
		for _, p := range b.Params {
			if !tracked(p) {
				continue
			}
			if bi == 0 {
				i := params[p]
				if i < len(f.Borrowed) && f.Borrowed[i] && !isVarParam(f, i) && (byRef == nil || !byRef[i]) {
					def(p, fRO)
				} else {
					def(p, fPoison)
				}
				continue
			}
			def(p, 0)
		}
		for _, v := range b.Instrs {
			if !tracked(v) {
				continue
			}
			switch {
			case v.Op == ssa.OpReuse:
				if _, ok := params[v.Args[0]]; ok {
					def(v, fW|fRoot)
				} else {
					def(v, fW)
				}
			case isUpdate(v):
				def(v, fW)
			default:
				def(v, fPoison)
			}
		}
	}
	// the joins
	reuseOf := map[*ssa.Value]*ssa.Value{}
	for bi, b := range f.Blocks {
		if bi > 0 {
			for k, p := range b.Params {
				if !tracked(p) {
					continue
				}
				for _, e := range b.Preds {
					if k < len(e.Args) && e.Args[k] != nil {
						union(p, e.Args[k])
					} else {
						flags[find(p)] |= fPoison
					}
				}
			}
		}
		for _, v := range b.Instrs {
			if !tracked(v) {
				continue
			}
			switch {
			case v.Op == ssa.OpReuse:
				a := v.Args[0]
				if _, ok := params[a]; ok {
					// every reuse of one parameter is one class
					if r, ok := reuseOf[a]; ok {
						union(v, r)
					} else {
						reuseOf[a] = v
					}
				} else {
					union(v, a)
				}
			case isUpdate(v):
				union(v, v.Args[0])
			}
		}
	}
	// the scopes, in the order of the accesses
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
			if !tracked(a) {
				continue
			}
			r := find(a)
			fl := flags[r]
			var k int
			switch {
			case fl&fPoison != 0:
				continue
			case fl&fRO != 0 && fl&fW == 0 && !w:
				if ro < 0 {
					if sc.n == scopeMax {
						continue
					}
					ro = sc.n
					sc.n++
				}
				k = ro
			case fl&fRO == 0 && fl&fRoot != 0:
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
