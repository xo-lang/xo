package ssa

import (
	"fmt"
	"strings"
)

// VerifyError lists every problem Verify found.
type VerifyError []string

func (e VerifyError) Error() string { return "ssa: " + strings.Join(e, "\n") }

// Verify checks the module's invariants: block structure (one terminator,
// last), edges (argument count and types match the target's parameters,
// predecessor lists agree with successor edges), dominance (every use is
// dominated by its definition; an edge argument is used at the end of
// its source block), and operand and result types of each instruction.
func (m *Module) Verify() error {
	var errs VerifyError
	for _, f := range m.Funcs {
		errs = append(errs, m.verifyFunc(f)...)
	}
	if len(errs) == 0 && m.RC {
		for _, f := range m.Funcs {
			errs = append(errs, m.verifyRC(f)...)
		}
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// VerifyFunc checks one function (see Verify).
func (m *Module) VerifyFunc(f *Func) error {
	errs := m.verifyFunc(f)
	if len(errs) == 0 && m.RC {
		errs = m.verifyRC(f)
	}
	if len(errs) > 0 {
		return VerifyError(errs)
	}
	return nil
}

func (m *Module) verifyFunc(f *Func) []string {
	var errs []string
	bad := func(format string, args ...any) {
		errs = append(errs, f.Name+": "+fmt.Sprintf(format, args...))
	}
	if len(f.Blocks) == 0 {
		bad("no blocks")
		return errs
	}
	inFunc := map[*Block]bool{}
	for _, b := range f.Blocks {
		inFunc[b] = true
	}
	if len(f.Blocks[0].Preds) > 0 {
		bad("entry block %s has predecessors", f.Blocks[0].Name())
	}
	// definitions: block and position
	type site struct {
		b   *Block
		idx int // -1 for a parameter
	}
	def := map[*Value]site{}
	for _, b := range f.Blocks {
		for _, p := range b.Params {
			if p.Op != OpParam || p.Block != b {
				bad("%s: parameter %s is malformed", b.Name(), p.Name())
			}
			def[p] = site{b, -1}
		}
		for i, v := range b.Instrs {
			if _, dup := def[v]; dup {
				bad("%s: value %s defined twice", b.Name(), v.Name())
			}
			def[v] = site{b, i}
			if v.Block != b {
				bad("%s: %s does not belong to its block", b.Name(), v.Name())
			}
		}
	}
	// structure and edges
	for _, b := range f.Blocks {
		if b.Term() == nil {
			bad("%s: does not end with a terminator", b.Name())
		}
		for i, v := range b.Instrs {
			if v.Op.IsTerminator() && i != len(b.Instrs)-1 {
				bad("%s: terminator %s is not last", b.Name(), v.Name())
			}
			if v.Op == OpParam {
				bad("%s: parameter %s in the instruction list", b.Name(), v.Name())
			}
			for _, e := range v.Succs {
				if e.From != b || !inFunc[e.To] {
					bad("%s: edge to %s is malformed", b.Name(), e.To.Name())
					continue
				}
				found := false
				for _, p := range e.To.Preds {
					found = found || p == e
				}
				if !found {
					bad("%s: edge to %s is missing from its predecessors", b.Name(), e.To.Name())
				}
				if len(e.Args) != len(e.To.Params) {
					bad("%s: edge to %s has %d arguments for %d parameters", b.Name(), e.To.Name(), len(e.Args), len(e.To.Params))
					continue
				}
				for k, a := range e.Args {
					if a.Type != e.To.Params[k].Type {
						bad("%s: edge to %s passes %s: %s for %s: %s", b.Name(), e.To.Name(), a.Name(), typeName(a.Type), e.To.Params[k].Name(), typeName(e.To.Params[k].Type))
					}
				}
			}
		}
		for _, e := range b.Preds {
			t := e.From.Term()
			found := false
			if t != nil {
				for _, s := range t.Succs {
					found = found || s == e
				}
			}
			if !found {
				bad("%s: predecessor edge from %s is not a successor edge", b.Name(), e.From.Name())
			}
		}
	}
	if len(errs) > 0 {
		return errs
	}
	// dominance
	idom := dominators(f)
	dominates := func(a, b *Block) bool {
		for b != nil {
			if a == b {
				return true
			}
			if idom[b] == b {
				return false
			}
			b = idom[b]
		}
		return false
	}
	usable := func(x *Value, at *Block, idx int) bool {
		d, ok := def[x]
		if !ok {
			return false
		}
		if _, reach := idom[at]; !reach {
			return true // unreachable code: nothing to check
		}
		if d.b == at {
			return d.idx < idx
		}
		return dominates(d.b, at)
	}
	for _, b := range f.Blocks {
		for i, v := range b.Instrs {
			for _, a := range v.Args {
				if a == nil {
					bad("%s: %s has a nil operand", b.Name(), v.Name())
				} else if !usable(a, b, i) {
					bad("%s: %s uses %s, which does not dominate it", b.Name(), v.Name(), a.Name())
				}
			}
			for _, e := range v.Succs {
				for _, a := range e.Args {
					if a == nil {
						bad("%s: edge to %s has a nil argument", b.Name(), e.To.Name())
					} else if !usable(a, b, len(b.Instrs)) {
						bad("%s: edge to %s passes %s, which does not dominate it", b.Name(), e.To.Name(), a.Name())
					}
				}
			}
			if msg := m.checkTypes(f, v); msg != "" {
				bad("%s: %s: %s", b.Name(), v.LongString(), msg)
			}
		}
	}
	return errs
}

func typeName(t *Type) string {
	if t == nil {
		return "<none>"
	}
	return t.Name
}

// checkTypes checks one instruction's operand and result types.
func (m *Module) checkTypes(f *Func, v *Value) string {
	arg := func(i int) *Type {
		if i < len(v.Args) && v.Args[i] != nil {
			return v.Args[i].Type
		}
		return nil
	}
	nargs := func(n int) string {
		if len(v.Args) != n {
			return fmt.Sprintf("want %d operands, have %d", n, len(v.Args))
		}
		return ""
	}
	boolT := m.Builtin(KBool)
	switch v.Op {
	case OpConst:
		want := map[string]Kind{"int64": KInt, "float64": KFloat, "bool": KBool, "string": KStr}[fmt.Sprintf("%T", v.Aux)]
		if v.Type == nil || v.Type.Kind != want {
			return "constant of the wrong type"
		}
	case OpAdd, OpSub, OpMul, OpDiv, OpRem:
		if s := nargs(2); s != "" {
			return s
		}
		if arg(0) != arg(1) || arg(0) != v.Type || (v.Type.Kind != KInt && v.Type.Kind != KFloat) {
			return "operands and result must be one numeric type"
		}
	case OpAnd, OpOr, OpXor, OpShl, OpShr:
		if s := nargs(2); s != "" {
			return s
		}
		if arg(0) != arg(1) || arg(0) != v.Type || (v.Type.Kind != KInt && v.Type.Kind != KBool) {
			return "operands and result must be one Int or Bool type"
		}
	case OpNeg, OpNot:
		if s := nargs(1); s != "" {
			return s
		}
		if arg(0) != v.Type {
			return "operand and result types differ"
		}
	case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
		if s := nargs(2); s != "" {
			return s
		}
		if arg(0) != arg(1) || v.Type != boolT {
			return "comparison of different types, or a result that is not Bool"
		}
	case OpIntToFloat:
		if nargs(1) != "" || arg(0).Kind != KInt || v.Type.Kind != KFloat {
			return "itof takes Int to Float"
		}
	case OpFloatToInt:
		if nargs(1) != "" || arg(0).Kind != KFloat || v.Type.Kind != KInt {
			return "ftoi takes Float to Int"
		}
	case OpStruct:
		if v.Type.Kind != KStruct || len(v.Args) != len(v.Type.Fields) {
			return "struct operands do not match the fields"
		}
		for i, a := range v.Args {
			if a.Type != v.Type.Fields[i].Type {
				return "field " + v.Type.Fields[i].Name + " has the wrong type"
			}
		}
	case OpUndef:
		return "undef in a reachable block"
	case OpTuple:
		if v.Type.Kind == KUnit && len(v.Args) == 0 {
			break
		}
		if v.Type.Kind != KTuple || len(v.Args) != len(v.Type.Elems) {
			return "tuple operands do not match the elements"
		}
		for i, a := range v.Args {
			if a.Type != v.Type.Elems[i] {
				return fmt.Sprintf("element %d has the wrong type", i)
			}
		}
	case OpVariant:
		tag, ok := v.Aux.(int)
		if !ok || v.Type.Kind != KEnum || tag < 0 || tag >= len(v.Type.Variants) {
			return "variant of a non enum or a bad tag"
		}
		fs := v.Type.Variants[tag].Fields
		if len(fs) != len(v.Args) {
			return "variant operands do not match its fields"
		}
		for i, a := range v.Args {
			if a.Type != fs[i].Type {
				return "variant field " + fs[i].Name + " has the wrong type"
			}
		}
	case OpField, OpSetField:
		i, ok := v.Aux.(int)
		if !ok || len(v.Args) == 0 {
			return "field needs an index and an operand"
		}
		var ft *Type
		switch t := arg(0); t.Kind {
		case KStruct:
			if i >= 0 && i < len(t.Fields) {
				ft = t.Fields[i].Type
			}
		case KTuple:
			if i >= 0 && i < len(t.Elems) {
				ft = t.Elems[i]
			}
		}
		if ft == nil {
			return "field index out of range or not a struct or tuple"
		}
		if v.Op == OpField && v.Type != ft {
			return "result type is not the field type"
		}
		if v.Op == OpSetField && (nargs(2) != "" || arg(1) != ft || v.Type != arg(0)) {
			return "setfield operands or result have the wrong type"
		}
	case OpTag:
		if nargs(1) != "" || arg(0).Kind != KEnum || v.Type.Kind != KInt {
			return "tag takes an enum to Int"
		}
	case OpPayload:
		r, ok := v.Aux.(PayloadRef)
		if !ok || nargs(1) != "" || arg(0).Kind != KEnum || r.Tag >= len(arg(0).Variants) || r.Field >= len(arg(0).Variants[r.Tag].Fields) {
			return "payload of a non enum or a bad reference"
		}
		if v.Type != arg(0).Variants[r.Tag].Fields[r.Field].Type {
			return "result type is not the payload field type"
		}
	case OpCall:
		name, _ := v.Aux.(string)
		if callee := m.Func(name); callee != nil {
			ps := callee.Params()
			if len(ps) != len(v.Args) {
				return fmt.Sprintf("call of %s with %d arguments for %d parameters", name, len(v.Args), len(ps))
			}
			for i, a := range v.Args {
				if a.Type != ps[i].Type {
					return fmt.Sprintf("argument %d has type %s, want %s", i, a.Type.Name, ps[i].Type.Name)
				}
			}
			want := callee.Results[0]
			if len(callee.Results) > 1 {
				want = m.Tuple(callee.Results...)
			}
			if v.Type != want {
				return "result type does not match the callee"
			}
		} else if _, ok := RuntimeCalls[name]; !ok && !strings.Contains(name, ".") {
			return "call of an unknown function"
		}
	case OpClosure:
		name, _ := v.Aux.(string)
		callee := m.Func(name)
		t := v.Type
		if callee == nil || !callee.Indirect || t == nil || t.Kind != KFunc {
			return "closure of a function that is not Indirect, or not of a function type"
		}
		ps := callee.Params()
		if len(ps) != len(t.Elems)+1 || ps[0].Type != t || len(callee.Results) != 1 || callee.Results[0] != t.Ret {
			return "closure type does not match its function"
		}
		for i, p := range ps[1:] {
			if p.Type != t.Elems[i] {
				return "closure type does not match its function"
			}
		}
		if len(v.Args) != len(callee.Captures) {
			return "closure captures do not match its function"
		}
		for i, a := range v.Args {
			if a.Type != callee.Captures[i] {
				return fmt.Sprintf("capture %d has the wrong type", i)
			}
		}
	case OpBox:
		if nargs(1) != "" || v.Type == nil || v.Type.Kind != KBox || v.Type.Elems[0] != arg(0) {
			return "box takes T to Box[T]"
		}
	case OpUnbox:
		if nargs(1) != "" || arg(0).Kind != KBox || v.Type != arg(0).Elems[0] {
			return "unbox takes Box[T] to T"
		}
	case OpCapture:
		i, ok := v.Aux.(int)
		if !ok || nargs(1) != "" || !f.Indirect || v.Args[0] != f.Params()[0] || i < 0 || i >= len(f.Captures) || v.Type != f.Captures[i] {
			return "capture must read the environment of its own function"
		}
	case OpCallValue:
		if len(v.Args) == 0 || arg(0).Kind != KFunc {
			return "callv needs a function value"
		}
		ft := arg(0)
		if len(v.Args)-1 != len(ft.Elems) || v.Type != ft.Ret {
			return "callv arguments or result do not match the function type"
		}
		for i, a := range v.Args[1:] {
			if a.Type != ft.Elems[i] {
				return fmt.Sprintf("callv argument %d has the wrong type", i)
			}
		}
	case OpBranch:
		if nargs(1) != "" || arg(0) != boolT || len(v.Succs) != 2 {
			return "br takes a Bool and two successors"
		}
	case OpJump:
		if len(v.Succs) != 1 {
			return "jump takes one successor"
		}
	case OpReturn:
		if len(v.Args) != len(f.Results) {
			return "ret does not return every result"
		}
		for i, a := range v.Args {
			if a.Type != f.Results[i] {
				return fmt.Sprintf("result %d has type %s, want %s", i, a.Type.Name, f.Results[i].Name)
			}
		}
	case OpFault:
		if nargs(1) != "" || arg(0).Kind != KStr {
			return "fault takes a Str"
		}
	case OpRetain, OpRelease, OpDrop, OpReuse, OpShare:
		if nargs(1) != "" || !arg(0).Heap() {
			return "reference counting of a value that is not on the heap"
		}
	}
	return ""
}

// dominators returns the immediate dominator of every block reachable
// from the entry (the entry maps to itself), by the iterative algorithm
// of Cooper, Harvey, and Kennedy.
func dominators(f *Func) map[*Block]*Block {
	var order []*Block // reverse postorder
	seen := map[*Block]bool{}
	var walk func(b *Block)
	walk = func(b *Block) {
		seen[b] = true
		if t := b.Term(); t != nil {
			for _, e := range t.Succs {
				if !seen[e.To] {
					walk(e.To)
				}
			}
		}
		order = append(order, b)
	}
	walk(f.Blocks[0])
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	num := map[*Block]int{}
	for i, b := range order {
		num[b] = i
	}
	idom := map[*Block]*Block{f.Blocks[0]: f.Blocks[0]}
	intersect := func(a, b *Block) *Block {
		for a != b {
			for num[a] > num[b] {
				a = idom[a]
			}
			for num[b] > num[a] {
				b = idom[b]
			}
		}
		return a
	}
	for changed := true; changed; {
		changed = false
		for _, b := range order[1:] {
			var nd *Block
			for _, e := range b.Preds {
				if _, ok := idom[e.From]; !ok || !seen[e.From] {
					continue
				}
				if nd == nil {
					nd = e.From
				} else {
					nd = intersect(e.From, nd)
				}
			}
			if nd != nil && idom[b] != nd {
				idom[b] = nd
				changed = true
			}
		}
	}
	return idom
}
