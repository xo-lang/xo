package ssa

import (
	"strconv"
	"strings"
)

// Reference counting with Perceus style ownership (decision 0046).
//
// InsertRC rewrites every function so that each heap object is owned
// exactly once on every path:
//
//   - Leaves. Structs and tuples are values, not heap cells, so ownership
//     is tracked per heap leaf: a Str, a List, or an enum with a heap
//     payload, addressed by a root value and a path of field indexes
//     (`v12.1.0`). A field read (`field`) of a struct or tuple is an alias
//     of its root's leaves, not a new value: taking a list out of the
//     tuple a `var` call returns, or out of a struct about to be rebuilt
//     with `setfield`, moves it without any count traffic.
//   - Roots. A value that is not such an alias is a root. Most roots own
//     their leaves at definition (call results, constructors, block
//     parameters, owned function parameters). Projections out of a heap
//     container (`list.index`, `list.get`, `payload`) borrow from it and
//     are retained when they live on. Borrowed parameters and string
//     constants are always alive and never released.
//   - Operands either borrow (the value must be alive) or consume (the
//     ownership moves). Constructors, `setfield`, edge arguments, `ret`,
//     list updates, and owned parameters of user functions consume;
//     everything else borrows.
//   - Insertion. A backward liveness pass over leaves finds each last
//     use: a consuming last use takes the ownership; any other consuming
//     use is preceded by `retain`; a borrowing last use is followed by
//     `release`; a dead definition is released at once. On a branch, a
//     leaf live into one successor but not the other is released on the
//     other edge (critical edges are split).
//   - Borrow inference. A parameter that no path consumes is borrowed:
//     the caller keeps it and the callee never retains or releases it.
//     Computed to a fixpoint over the call graph.
//   - Cancellation. `retain x ... release x` in one block with nothing
//     between that consumes or releases x or what x was projected from
//     is removed.
//   - Reuse. A list update (`list.set`, `list.push`, `list.pop`) needs a
//     uniquely owned list to write in place. Where the list is provably
//     unique (fresh from a list operation, never retained, through block
//     parameters to a fixpoint) nothing is added; elsewhere `reuse`
//     checks the count at run time and copies only when it is shared.

// leaf is one reference counted component of a value.
type leaf struct {
	root *Value
	path string // "" or ".1.0": field indexes through structs and tuples
}

func (l leaf) String() string { return l.root.Name() + l.path }

type mode int

const (
	mNone mode = iota
	mBorrow
	mConsume
)

type rootKind int

const (
	rkOwned    rootKind = iota // owns its leaves from its definition
	rkBorrowed                 // a borrowed parameter or a constant: always alive, never released
	rkProj                     // a projection out of a heap container: alive while the container is
)

// use is one leaf an instruction uses, and how.
type use struct {
	l leaf
	m mode
}

// runtimeModes are the operand modes of the runtime calls whose heap
// operands are consumed or that matter to RC. Others borrow.
var runtimeModes = map[string][]mode{
	// str.concat consumes its left operand: the runtime appends in place
	// when it is unique (s = s + x in a loop is linear)
	"str.concat":       {mConsume, mBorrow},
	"str.concat_int":   {mConsume, mNone},
	"str.concat_float": {mConsume, mNone},
	"list.set":         {mConsume, mNone, mConsume},
	"list.push":        {mConsume, mConsume},
	"list.pop":         {mConsume},
}

// projCalls are the runtime calls whose result is a projection out of
// their first operand.
var projCalls = map[string]bool{"list.index": true, "list.get": true}

// updateCalls are the list updates that need a unique list (reuse).
var updateCalls = map[string]bool{"list.set": true, "list.push": true, "list.pop": true}

func decomposable(t *Type) bool { return t.Kind == KStruct || t.Kind == KTuple }

func fieldType(t *Type, i int) *Type {
	if t.Kind == KStruct {
		return t.Fields[i].Type
	}
	return t.Elems[i]
}

func numFields(t *Type) int {
	if t.Kind == KStruct {
		return len(t.Fields)
	}
	return len(t.Elems)
}

// rcInfo is the module wide state of the RC pass and of its verifier.
type rcInfo struct {
	m     *Module
	paths map[*Type][]string
	funcs map[string]*Func
}

func newRCInfo(m *Module) *rcInfo {
	r := &rcInfo{m: m, paths: map[*Type][]string{}, funcs: map[string]*Func{}}
	for _, f := range m.Funcs {
		r.funcs[f.Name] = f
	}
	return r
}

// leafPaths returns the paths of the heap leaves of a value of type t.
func (r *rcInfo) leafPaths(t *Type) []string {
	if t == nil {
		return nil
	}
	if p, ok := r.paths[t]; ok {
		return p
	}
	var out []string
	if t.Heap() {
		if decomposable(t) {
			for i := 0; i < numFields(t); i++ {
				for _, p := range r.leafPaths(fieldType(t, i)) {
					out = append(out, "."+strconv.Itoa(i)+p)
				}
			}
		} else {
			out = []string{""}
		}
	}
	r.paths[t] = out
	return out
}

// isAlias reports a field read of a struct or tuple: a name for some of
// its operand's leaves.
func isAlias(v *Value) bool {
	return v.Op == OpField && decomposable(v.Args[0].Type)
}

// base returns the root and path that v denotes.
func base(v *Value) (*Value, string) {
	if isAlias(v) {
		r, p := base(v.Args[0])
		return r, p + "." + strconv.Itoa(v.Aux.(int))
	}
	return v, ""
}

// leaves returns the heap leaves v denotes.
func (r *rcInfo) leaves(v *Value) []leaf {
	root, p := base(v)
	var out []leaf
	for _, lp := range r.leafPaths(v.Type) {
		out = append(out, leaf{root, p + lp})
	}
	return out
}

// isRoot reports whether v is a root with heap leaves.
func isRoot(v *Value) bool {
	return v.Type != nil && !v.Op.IsTerminator() && v.Op != OpRetain && v.Op != OpRelease && !isAlias(v) && v.Type.Heap()
}

func paramIndex(v *Value) int {
	for i, p := range v.Block.Params {
		if p == v {
			return i
		}
	}
	return -1
}

func (r *rcInfo) kind(v *Value) rootKind {
	switch v.Op {
	case OpParam:
		f := v.Block.Func
		if v.Block == f.Blocks[0] {
			if i := paramIndex(v); i >= 0 && i < len(f.Borrowed) && f.Borrowed[i] {
				return rkBorrowed
			}
		}
	case OpConst:
		return rkBorrowed
	case OpPayload, OpCapture, OpUnbox:
		return rkProj
	case OpCall:
		if name, _ := v.Aux.(string); projCalls[name] {
			return rkProj
		}
	}
	return rkOwned
}

// parent is the leaf a projection root was taken from.
func (r *rcInfo) parent(v *Value) leaf {
	return r.leaves(v.Args[0])[0]
}

func (r *rcInfo) tracked(l leaf) bool { return r.kind(l.root) != rkBorrowed }

// modes returns the operand modes of v (not of its edge arguments).
func (r *rcInfo) modes(v *Value) []mode {
	ms := make([]mode, len(v.Args))
	set := func(m mode) {
		for i := range ms {
			ms[i] = m
		}
	}
	switch v.Op {
	case OpStruct, OpTuple, OpVariant, OpReturn, OpRelease, OpReuse, OpClosure, OpBox:
		set(mConsume)
	case OpFault, OpRetain, OpEq, OpNe, OpCapture, OpUnbox:
		set(mBorrow)
	case OpCallValue:
		set(mConsume)
		ms[0] = mBorrow
	case OpPayload:
		if v.Type.Heap() {
			set(mBorrow)
		}
	case OpSetField:
		ms[0], ms[1] = mConsume, mConsume // operand 0 except field Aux; see uses
	case OpCall:
		name, _ := v.Aux.(string)
		if callee := r.funcs[name]; callee != nil {
			for i := range ms {
				ms[i] = mConsume
				if i < len(callee.Borrowed) && callee.Borrowed[i] {
					ms[i] = mBorrow
				}
			}
		} else if rm, ok := runtimeModes[name]; ok {
			copy(ms, rm)
		} else {
			set(mBorrow)
		}
	}
	return ms
}

// uses returns the heap leaves v uses, in operand order (edge arguments
// excluded).
func (r *rcInfo) uses(v *Value) []use {
	var out []use
	for i, m := range r.modes(v) {
		if m == mNone {
			continue
		}
		a := v.Args[i]
		ls := r.leaves(a)
		if v.Op == OpSetField && i == 0 {
			// the replaced field is not used: it was released at its own
			// last use (or moved out by an alias)
			_, p := base(a)
			pre := p + "." + strconv.Itoa(v.Aux.(int))
			for _, l := range ls {
				if l.path == pre || strings.HasPrefix(l.path, pre+".") {
					continue
				}
				out = append(out, use{l, m})
			}
			continue
		}
		for _, l := range ls {
			out = append(out, use{l, m})
		}
	}
	return out
}

// edgeUses returns the leaves the arguments of e pass (consumed).
func (r *rcInfo) edgeUses(e *Edge) []leaf {
	var out []leaf
	for _, a := range e.Args {
		out = append(out, r.leaves(a)...)
	}
	return out
}

// reuseVarParams makes each `var` List parameter of a function that
// updates lists unique once, at entry (`reuse`), instead of at the first
// update of every loop iteration: the parameter reaches the loops through
// block parameters, which are unique only when every incoming list is.
func (f *Func) reuseVarParams() {
	updates := false
	for _, b := range f.Blocks {
		for _, v := range b.Instrs {
			if name, ok := v.Aux.(string); ok && v.Op == OpCall && updateCalls[name] {
				updates = true
			}
		}
	}
	if !updates {
		return
	}
	entry := f.Blocks[0]
	var head []*Value
	for _, i := range f.VarParams {
		p := entry.Params[i]
		if p.Type.Kind != KList {
			continue
		}
		u := f.newValue(OpReuse, p.Type, []*Value{p}, nil)
		u.Block = entry
		head = append(head, u)
		for _, b := range f.Blocks {
			for _, v := range b.Instrs {
				for k, a := range v.Args {
					if a == p {
						v.Args[k] = u
					}
				}
				for _, e := range v.Succs {
					for k, a := range e.Args {
						if a == p {
							e.Args[k] = u
						}
					}
				}
			}
		}
	}
	entry.Instrs = append(head, entry.Instrs...)
}

// ---------------------------------------------------------------------------
// Borrow inference

// inferBorrows marks the parameters no path of their function consumes.
// `var` parameters are owned (they are returned).
func (r *rcInfo) inferBorrows() {
	for _, f := range r.m.Funcs {
		ps := f.Params()
		f.Borrowed = make([]bool, len(ps))
		isVar := map[int]bool{}
		for _, i := range f.VarParams {
			isVar[i] = true
		}
		for i, p := range ps {
			f.Borrowed[i] = p.Type.Heap() && !isVar[i]
			if f.Indirect {
				// callers through a function value pass the closure
				// borrowed and the arguments owned
				f.Borrowed[i] = i == 0
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, f := range r.m.Funcs {
			consumed := func(l leaf) {
				p := l.root
				if p.Op != OpParam || p.Block != f.Blocks[0] {
					return
				}
				if i := paramIndex(p); i >= 0 && f.Borrowed[i] {
					f.Borrowed[i] = false
					changed = true
				}
			}
			for _, b := range f.Blocks {
				for _, v := range b.Instrs {
					for _, u := range r.uses(v) {
						if u.m == mConsume {
							consumed(u.l)
						}
					}
					for _, e := range v.Succs {
						for _, l := range r.edgeUses(e) {
							consumed(l)
						}
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Per function pass

type leafSet map[leaf]bool

type rcFunc struct {
	r       *rcInfo
	f       *Func
	liveIn  map[*Block]leafSet
	liveOut map[*Block]leafSet
	pre     map[*Block][]*Value // code for the start of a block (edges)
}

// InsertRC inserts reference counting into every function of m (see the
// comment at the top of rc.go) and sets m.RC. Run it on a verified
// module; Verify then also checks that ownership balances on every path.
func (m *Module) InsertRC() {
	r := newRCInfo(m)
	for _, f := range m.Funcs {
		f.reuseVarParams()
	}
	r.inferBorrows()
	for _, f := range m.Funcs {
		c := &rcFunc{r: r, f: f, pre: map[*Block][]*Value{}}
		c.liveness()
		c.insert()
		c.cancel()
		c.reuse()
		f.renumber()
	}
	m.RC = true
}

func (c *rcFunc) liveness() {
	f := c.f
	c.liveIn = map[*Block]leafSet{}
	c.liveOut = map[*Block]leafSet{}
	used := map[*Block]leafSet{}
	for _, b := range f.Blocks {
		u := leafSet{}
		for _, v := range b.Instrs {
			for _, x := range c.r.uses(v) {
				if c.r.tracked(x.l) {
					u[x.l] = true
				}
			}
			for _, e := range v.Succs {
				for _, l := range c.r.edgeUses(e) {
					if c.r.tracked(l) {
						u[l] = true
					}
				}
			}
		}
		used[b] = u
		c.liveIn[b] = leafSet{}
	}
	for changed := true; changed; {
		changed = false
		for i := len(f.Blocks) - 1; i >= 0; i-- {
			b := f.Blocks[i]
			out := leafSet{}
			if t := b.Term(); t != nil {
				for _, e := range t.Succs {
					for l := range c.liveIn[e.To] {
						out[l] = true
					}
				}
			}
			in := leafSet{}
			for l := range used[b] {
				if l.root.Block != b {
					in[l] = true
				}
			}
			for l := range out {
				if l.root.Block != b {
					in[l] = true
				}
			}
			c.liveOut[b] = out
			if len(in) != len(c.liveIn[b]) {
				changed = true
			}
			c.liveIn[b] = in
		}
	}
}

// mk creates an instruction in block b without placing it.
func (c *rcFunc) mk(b *Block, op Op, t *Type, aux any, args ...*Value) *Value {
	v := c.f.newValue(op, t, args, aux)
	v.Block = b
	return v
}

// leafValue returns the instructions that materialize leaf l in block b
// (field reads from its root) and the value.
func (c *rcFunc) leafValue(l leaf, b *Block) ([]*Value, *Value) {
	v := l.root
	var code []*Value
	if l.path == "" {
		return nil, v
	}
	for _, s := range strings.Split(l.path[1:], ".") {
		i, _ := strconv.Atoi(s)
		v = c.mk(b, OpField, fieldType(v.Type, i), i, v)
		code = append(code, v)
	}
	return code, v
}

func (c *rcFunc) rc(op Op, l leaf, b *Block, n int) []*Value {
	var out []*Value
	for k := 0; k < n; k++ {
		code, v := c.leafValue(l, b)
		out = append(out, code...)
		out = append(out, c.mk(b, op, nil, nil, v))
	}
	return out
}

// countLeaves counts the leaves of a list, keeping first appearance
// order.
type counted struct {
	order []leaf
	n     map[leaf][2]int // consumes, borrows
}

func (k *counted) add(l leaf, m mode) {
	if k.n == nil {
		k.n = map[leaf][2]int{}
	}
	x, ok := k.n[l]
	if !ok {
		k.order = append(k.order, l)
	}
	if m == mConsume {
		x[0]++
	} else {
		x[1]++
	}
	k.n[l] = x
}

func (c *rcFunc) insert() {
	blocks := append([]*Block(nil), c.f.Blocks...)
	for _, b := range blocks {
		c.walk(b)
	}
	for b, code := range c.pre {
		b.Instrs = append(append([]*Value(nil), code...), b.Instrs...)
	}
}

// edgeNeed counts what edge e needs held: its arguments' leaves and the
// leaves live into its target.
func (c *rcFunc) edgeNeed(e *Edge) *counted {
	k := &counted{}
	for _, l := range c.r.edgeUses(e) {
		k.add(l, mConsume)
	}
	for _, l := range sortedLeaves(c.liveIn[e.To]) {
		k.add(l, mConsume)
	}
	return k
}

// sortedLeaves orders a leaf set by root ID and path, for stable output.
func sortedLeaves(s leafSet) []leaf {
	out := make([]leaf, 0, len(s))
	for l := range s {
		out = append(out, l)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && leafLess(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func leafLess(a, b leaf) bool {
	if a.root.ID != b.root.ID {
		return a.root.ID < b.root.ID
	}
	return a.path < b.path
}

// walk inserts RC into block b, backward from its terminator.
func (c *rcFunc) walk(b *Block) {
	r := c.r
	term := b.Term()
	body := b.Instrs[:len(b.Instrs)-1]
	live := leafSet{}
	var rev []*Value // the new instruction list, reversed
	switch term.Op {
	case OpJump:
		need := c.edgeNeed(term.Succs[0])
		var code []*Value
		for _, l := range need.order {
			n := need.n[l][0]
			if r.tracked(l) {
				live[l] = true
				n--
			}
			code = append(code, c.rc(OpRetain, l, b, n)...)
		}
		rev = append(rev, term)
		for i := len(code) - 1; i >= 0; i-- {
			rev = append(rev, code[i])
		}
	case OpBranch:
		needs := make([]*counted, len(term.Succs))
		for i, e := range term.Succs {
			needs[i] = c.edgeNeed(e)
			for _, l := range needs[i].order {
				if r.tracked(l) {
					live[l] = true
				}
			}
		}
		held := sortedLeaves(live)
		for i, e := range term.Succs {
			var code func(at *Block) []*Value
			code = func(at *Block) []*Value {
				var out []*Value
				for _, l := range held {
					switch n := needs[i].n[l][0]; {
					case n == 0:
						out = append(out, c.rc(OpRelease, l, at, 1)...)
					case n > 1:
						out = append(out, c.rc(OpRetain, l, at, n-1)...)
					}
				}
				for _, l := range needs[i].order {
					if !r.tracked(l) {
						out = append(out, c.rc(OpRetain, l, at, needs[i].n[l][0])...)
					}
				}
				return out
			}
			c.edgeCode(e, code)
		}
		rev = append(rev, term)
	case OpReturn:
		k := &counted{}
		for _, a := range term.Args {
			for _, l := range r.leaves(a) {
				k.add(l, mConsume)
			}
		}
		var code []*Value
		for _, l := range k.order {
			n := k.n[l][0]
			if r.tracked(l) {
				live[l] = true
				n--
			}
			code = append(code, c.rc(OpRetain, l, b, n)...)
		}
		rev = append(rev, term)
		for i := len(code) - 1; i >= 0; i-- {
			rev = append(rev, code[i])
		}
	case OpFault:
		// the program stops: nothing is released
		for _, u := range r.uses(term) {
			if r.tracked(u.l) {
				live[u.l] = true
			}
		}
		rev = append(rev, term)
	default:
		rev = append(rev, term)
	}
	for k := len(body) - 1; k >= 0; k-- {
		v := body[k]
		var after []*Value
		if isRoot(v) {
			kd := r.kind(v)
			for _, l := range r.leaves(v) {
				switch {
				case kd == rkBorrowed:
				case live[l] && kd == rkProj:
					after = append(after, c.rc(OpRetain, l, b, 1)...)
				case !live[l] && kd == rkOwned:
					after = append(after, c.rc(OpRelease, l, b, 1)...)
				}
				delete(live, l)
			}
		}
		k2 := &counted{}
		for _, u := range r.uses(v) {
			k2.add(u.l, u.m)
		}
		var before []*Value
		for _, l := range k2.order {
			n := k2.n[l]
			switch {
			case !r.tracked(l) || live[l]:
				before = append(before, c.rc(OpRetain, l, b, n[0])...)
			case n[1] > 0:
				before = append(before, c.rc(OpRetain, l, b, n[0])...)
				after = append(after, c.rc(OpRelease, l, b, 1)...)
			default:
				before = append(before, c.rc(OpRetain, l, b, n[0]-1)...)
			}
			if r.tracked(l) {
				live[l] = true
			}
		}
		for i := len(after) - 1; i >= 0; i-- {
			rev = append(rev, after[i])
		}
		rev = append(rev, v)
		for i := len(before) - 1; i >= 0; i-- {
			rev = append(rev, before[i])
		}
	}
	// parameters: release the ones no one uses
	var start []*Value
	for _, p := range b.Params {
		if !isRoot(p) {
			continue
		}
		kd := r.kind(p)
		for _, l := range r.leaves(p) {
			if !live[l] && kd == rkOwned {
				start = append(start, c.rc(OpRelease, l, b, 1)...)
			}
			delete(live, l)
		}
	}
	out := start
	for i := len(rev) - 1; i >= 0; i-- {
		out = append(out, rev[i])
	}
	b.Instrs = out
}

// edgeCode places the code an edge needs: at the start of its target
// when the target has no other predecessor and takes no arguments,
// otherwise in a new block that splits the edge.
func (c *rcFunc) edgeCode(e *Edge, code func(at *Block) []*Value) {
	to := e.To
	if t := to.Term(); t != nil && (t.Op == OpUnreachable || t.Op == OpFault) {
		// the program stops there: nothing needs releasing
		return
	}
	if len(to.Preds) == 1 && len(e.Args) == 0 {
		if x := code(to); len(x) > 0 {
			c.pre[to] = append(c.pre[to], x...)
		}
		return
	}
	n := c.f.NewBlock()
	x := code(n)
	if len(x) == 0 {
		// nothing to do: undo the block
		c.f.Blocks = c.f.Blocks[:len(c.f.Blocks)-1]
		c.f.nextBlock--
		return
	}
	n.Instrs = x
	j := c.mk(n, OpJump, nil, nil)
	ne := &Edge{From: n, To: to, Args: e.Args}
	j.Succs = []*Edge{ne}
	n.Instrs = append(n.Instrs, j)
	for i, p := range to.Preds {
		if p == e {
			to.Preds[i] = ne
		}
	}
	e.To = n
	e.Args = nil
	n.Preds = []*Edge{e}
}

// ---------------------------------------------------------------------------
// Cancellation

// anchors returns the roots whose consumption could free what root v
// refers to: v, and for a projection the container it came from.
func (r *rcInfo) anchors(v *Value) map[*Value]bool {
	out := map[*Value]bool{}
	for {
		out[v] = true
		if r.kind(v) != rkProj {
			return out
		}
		v = r.parent(v).root
	}
}

// cancel removes `retain x ... release x` pairs within a block when
// nothing between consumes or releases x or its anchors.
func (c *rcFunc) cancel() {
	r := c.r
	for _, b := range c.f.Blocks {
		for changed := true; changed; {
			changed = false
			for j, v := range b.Instrs {
				if v.Op != OpRelease {
					continue
				}
				key := r.leaves(v.Args[0])[0]
				an := r.anchors(key.root)
				for k := j - 1; k >= 0; k-- {
					w := b.Instrs[k]
					if w.Op == OpRetain && r.leaves(w.Args[0])[0] == key {
						b.Instrs = append(b.Instrs[:k:k], append(b.Instrs[k+1:j:j], b.Instrs[j+1:]...)...)
						changed = true
						break
					}
					if w == key.root || c.touches(w, an) {
						break
					}
				}
				if changed {
					break
				}
			}
		}
	}
	c.f.removeDeadFields()
}

// removeDeadFields deletes field reads whose result is unused (the RC
// pass makes them to name leaves; cancellation leaves some unused).
func (f *Func) removeDeadFields() {
	for changed := true; changed; {
		changed = false
		used := map[*Value]bool{}
		for _, b := range f.Blocks {
			for _, v := range b.Instrs {
				for _, a := range v.Args {
					used[a] = true
				}
				for _, e := range v.Succs {
					for _, a := range e.Args {
						used[a] = true
					}
				}
			}
		}
		for _, b := range f.Blocks {
			out := b.Instrs[:0]
			for _, v := range b.Instrs {
				if v.Op == OpField && !used[v] {
					changed = true
					continue
				}
				out = append(out, v)
			}
			b.Instrs = out
		}
	}
}

// touches reports whether w consumes, retains, releases, or reuses a
// leaf of one of the roots in an.
func (c *rcFunc) touches(w *Value, an map[*Value]bool) bool {
	r := c.r
	switch w.Op {
	case OpRetain, OpRelease, OpReuse:
		root, _ := base(w.Args[0])
		return an[root]
	}
	for _, u := range r.uses(w) {
		if u.m == mConsume && an[u.l.root] {
			return true
		}
	}
	for _, e := range w.Succs {
		for _, l := range r.edgeUses(e) {
			if an[l.root] {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Uniqueness and reuse

// unique computes which List leaves are provably unique wherever they
// are used: made fresh by a list operation or reuse, moved through
// constructors and block parameters, and never retained.
func (r *rcInfo) unique(f *Func) func(l leaf) bool {
	retained := map[leaf]bool{}
	for _, b := range f.Blocks {
		for _, v := range b.Instrs {
			if v.Op == OpRetain {
				for _, l := range r.leaves(v.Args[0]) {
					retained[l] = true
				}
			}
		}
	}
	param := map[leaf]bool{}
	var uniq func(l leaf) bool
	uniq = func(l leaf) bool {
		if retained[l] || l.root.Type == nil {
			return false
		}
		v := l.root
		head, rest := l.path, ""
		if head != "" {
			if i := strings.Index(head[1:], "."); i >= 0 {
				head, rest = l.path[:i+1], l.path[i+1:]
			}
		}
		idx := -1
		if head != "" {
			idx, _ = strconv.Atoi(head[1:])
		}
		switch v.Op {
		case OpCall:
			switch v.Aux.(string) {
			case "list.new", "list.set", "list.push":
				return l.path == ""
			case "list.pop":
				return l.path == ".0"
			}
		case OpReuse:
			return l.path == ""
		case OpStruct, OpTuple:
			if idx >= 0 {
				ar, ap := base(v.Args[idx])
				return uniq(leaf{ar, ap + rest})
			}
		case OpSetField:
			if idx == v.Aux.(int) {
				ar, ap := base(v.Args[1])
				return uniq(leaf{ar, ap + rest})
			}
			ar, ap := base(v.Args[0])
			return uniq(leaf{ar, ap + l.path})
		case OpParam:
			if v.Block == f.Blocks[0] {
				return false
			}
			return param[l]
		}
		return false
	}
	// block parameters: optimistic, then falsified to a fixpoint
	var ps []leaf
	for _, b := range f.Blocks[1:] {
		for _, p := range b.Params {
			for _, l := range r.leaves(p) {
				param[l] = true
				ps = append(ps, l)
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, l := range ps {
			if !param[l] {
				continue
			}
			i := paramIndex(l.root)
			for _, e := range l.root.Block.Preds {
				ar, ap := base(e.Args[i])
				if !uniq(leaf{ar, ap + l.path}) {
					param[l] = false
					changed = true
					break
				}
			}
		}
	}
	return uniq
}

// reuse puts `reuse` before each list update whose list is not provably
// unique.
func (c *rcFunc) reuse() {
	r := c.r
	uniq := r.unique(c.f)
	for _, b := range c.f.Blocks {
		var out []*Value
		for _, v := range b.Instrs {
			if v.Op == OpCall && updateCalls[v.Aux.(string)] {
				a := v.Args[0]
				if !uniq(r.leaves(a)[0]) {
					u := c.mk(b, OpReuse, a.Type, nil, a)
					out = append(out, u)
					v.Args[0] = u
				}
			}
			out = append(out, v)
		}
		b.Instrs = out
	}
}
