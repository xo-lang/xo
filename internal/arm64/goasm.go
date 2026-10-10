package arm64

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ssa"
)

// Go mode (decision 0084, `xo build --hybrid`): hot leaf functions of a
// Go backend build emitted as Go assembler (Plan 9 syntax, ABI0) by this
// backend's instruction selection and register allocation, linked into
// the Go program, so they run at native code speed while the program
// keeps the Go runtime (net/http, TLS, the scheduler, the GC).
//
// What changes from the native target:
//
//   - A function is eligible when it allocates nothing and touches no
//     reference count: its parameters and results are Int, Float, Bool,
//     structs of Int and Float fields, and Lists of those (a Go slice
//     passed in: base, length, capacity); it calls only other eligible
//     functions and the scalar and List element methods instruction
//     selection lowers inline. A `var` List parameter is written in
//     place (the Go wrapper makes it unique first, as the Go backend does
//     before an element write); any other List is only read.
//   - A List value is the address of its first element (the Go slice's
//     base), with its length a separate register (goLens): there is no
//     header in memory.
//   - Arguments and results are in memory (ABI0): `p0+0(FP)` in the
//     callee, 8(RSP) upward in the caller. Go saves no register across a
//     call, so values live across one are spilled; x18, x26, x27 (the
//     assembler's temporary), x28 (g), x29, and x30 are never allocated.
//   - Go's assembler writes the prologue: a function that calls another
//     Xo function gets the stack check (deep recursion grows the
//     goroutine stack); one that calls nothing but the fault path is
//     NOSPLIT. Frames hold no pointers the GC must see (NO_LOCAL_POINTERS):
//     a List's base in a spill slot is kept alive by the argument frame,
//     and Go heap objects do not move.
//   - A fault (overflow, index, division) calls the Go function
//     xoHybridFault(kind, a, b, c, pos) of the generated package, which
//     raises the fault the Go backend raises (rt.HybridFault).
//
// The functions are emitted in GNU syntax as for the native target, then
// translated line by line (gosyntax.go).

// GoOptions configure GenerateGo.
type GoOptions struct {
	// All selects every eligible function, not only the hot ones (those
	// with a loop; a call heavy function without one, such as recursive
	// fib, is faster as Go: ABI0 passes arguments in memory and every
	// call runs Go's stack check prologue): differential tests.
	All bool
	// NoInline turns off the inlining of small leaf functions.
	NoInline bool
}

// GoFunc is one function emitted as Go assembler.
type GoFunc struct {
	Name string // the SSA name (a function of the main module)
	Sym  string // the Go identifier of the assembly function
	// Wrap: selected, so the Go backend's function calls the assembly
	// one; otherwise it is emitted only for other assembly functions.
	Wrap bool
	// NoEscape: the function calls no Xo function (it never yields with
	// a List's address in a register), so //go:noescape is right.
	NoEscape bool
	// Unshare lists the `var` List parameters the function writes: the
	// wrapper makes each the unique owner of its elements first.
	Unshare []int
	NoSplit bool
	Frame   int
	ArgSize int
}

// GoProgram is the Go assembler of the eligible functions of a module.
type GoProgram struct {
	// Asm is the .s file (empty when no function was emitted).
	Asm   []byte
	Funcs map[string]*GoFunc
	// Rejected gives the reason each function of the module is not
	// emitted.
	Rejected map[string]string
	// Positions are the fault positions, indexed by the pos argument of
	// xoHybridFault.
	Positions []string
}

// GoSym is the Go identifier of the assembly function of Xo function n.
func GoSym(n string) string {
	var b strings.Builder
	b.WriteString("xoasm_")
	for i := 0; i < len(n); i++ {
		c := n[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			b.WriteByte(c)
		case c == '_':
			b.WriteString("__")
		default:
			fmt.Fprintf(&b, "_%02x", c)
		}
	}
	return b.String()
}

func goSym(n string) string { return GoSym(n) }

// goSlot is the place of a word in a function's ABI0 argument frame.
type goSlot struct {
	name string
	off  int
	size int // 8, or 1 for a Bool
}

// goSig is a function's ABI0 frame: per parameter the slots of its words
// (a List: base and length; its capacity follows the length), the
// slots of the first result's words, and the frame's size.
type goSig struct {
	params [][]goSlot
	res    []goSlot
	size   int
}

func alignUp(n, a int) int { return (n + a - 1) / a * a }

// goFlat returns the slots of a value of type t (Int, Float, Bool, or a
// struct of Int and Float fields) at offset 0, its size and alignment.
func goFlat(t *ssa.Type, name string, top bool) ([]goSlot, int, int, error) {
	switch t.Kind {
	case ssa.KInt:
		if t.Bits != 0 {
			return nil, 0, 0, fmt.Errorf("a sized integer `%s`", t.Name)
		}
		return []goSlot{{name, 0, 8}}, 8, 8, nil
	case ssa.KFloat:
		return []goSlot{{name, 0, 8}}, 8, 8, nil
	case ssa.KBool:
		if !top {
			return nil, 0, 0, fmt.Errorf("a Bool field in `%s`", name)
		}
		return []goSlot{{name, 0, 1}}, 1, 1, nil
	case ssa.KStruct:
		if t.Coll {
			return nil, 0, 0, fmt.Errorf("a collection `%s`", t.Name)
		}
		var out []goSlot
		off := 0
		for _, f := range t.Fields {
			sl, size, _, err := goFlat(f.Type, name+"_F_"+f.Name, false)
			if err != nil {
				return nil, 0, 0, err
			}
			for _, s := range sl {
				out = append(out, goSlot{s.name, off + s.off, s.size})
			}
			off += size
		}
		if off == 0 {
			return nil, 0, 0, fmt.Errorf("an empty struct `%s`", t.Name)
		}
		return out, off, 8, nil
	}
	return nil, 0, 0, fmt.Errorf("a `%s` at the Go boundary", t.Name)
}

// goElem reports whether t is a List element type of Go mode: the Go
// backend's element layout is the native one (8 byte words, a Bool one
// byte on its own).
func goElem(t *ssa.Type) bool {
	if t.Kind == ssa.KBool {
		return true
	}
	_, _, _, err := goFlat(t, "e", false)
	return err == nil
}

func (g *gen) goSigOf(f *ssa.Func) (*goSig, error) {
	sig := &goSig{}
	off := 0
	for i, p := range f.Params() {
		name := fmt.Sprintf("p%d", i)
		if p.Type.Kind == ssa.KList {
			if !goElem(p.Type.Elems[0]) {
				return nil, fmt.Errorf("a `%s` parameter", p.Type.Name)
			}
			off = alignUp(off, 8)
			sig.params = append(sig.params, []goSlot{{name + "_base", off, 8}, {name + "_len", off + 8, 8}})
			off += 24
			continue
		}
		sl, size, al, err := goFlat(p.Type, name, true)
		if err != nil {
			return nil, err
		}
		off = alignUp(off, al)
		var ps []goSlot
		for _, s := range sl {
			ps = append(ps, goSlot{s.name, off + s.off, s.size})
		}
		sig.params = append(sig.params, ps)
		off += size
	}
	off = alignUp(off, 8)
	if r := f.Results[0]; r.Kind != ssa.KUnit {
		sl, size, al, err := goFlat(r, "r0", true)
		if err != nil {
			return nil, err
		}
		off = alignUp(off, al)
		for _, s := range sl {
			sig.res = append(sig.res, goSlot{s.name, off + s.off, s.size})
		}
		off += size
	}
	// Go's argument size is not rounded up after a Bool result (go vet
	// -asmdecl: `func f(n int64) bool` is $...-9)
	sig.size = off
	return sig, nil
}

// goValue reports whether values of type t can live in registers of a
// Go mode function.
func goValue(t *ssa.Type) bool {
	switch t.Kind {
	case ssa.KUnit, ssa.KFloat, ssa.KBool:
		return true
	case ssa.KInt:
		return t.Bits == 0
	case ssa.KStruct:
		if t.Coll {
			return false
		}
		for _, f := range t.Fields {
			if !goValue(f.Type) {
				return false
			}
		}
		return true
	case ssa.KTuple:
		for _, e := range t.Elems {
			if !goValue(e) {
				return false
			}
		}
		return true
	case ssa.KEnum:
		if t.Heap() {
			return false
		}
		for _, v := range t.Variants {
			for _, f := range v.Fields {
				if !goValue(f.Type) {
					return false
				}
			}
		}
		return true
	case ssa.KList:
		return goElem(t.Elems[0])
	}
	return false
}

// goMethods are the calls instruction selection lowers inline.
var goMethods = map[string]bool{
	"list.index": true, "list.set": true, "list.len": true, "list.get": true,
	"Int.min": true, "Int.max": true, "Int.abs": true,
	"Float.sqrt": true, "Float.floor": true, "Float.ceil": true, "Float.trunc": true, "Float.round": true,
	"Float.abs": true, "Float.min": true, "Float.max": true, "Float.is_nan": true,
}

// goOrigins maps each List value of f to the parameter it is (through
// reuse, element writes, block parameters, and the `var` results of
// calls), or -1 when it is not one parameter.
func goOrigins(f *ssa.Func, byName map[string]*ssa.Func) map[*ssa.Value]int {
	const top = -2
	org := map[*ssa.Value]int{}
	for i, p := range f.Params() {
		org[p] = i
	}
	meet := func(a, b int) int {
		switch {
		case a == top:
			return b
		case b == top:
			return a
		case a == b:
			return a
		}
		return -1
	}
	get := func(v *ssa.Value) int {
		if o, ok := org[v]; ok {
			return o
		}
		return top
	}
	for changed := true; changed; {
		changed = false
		set := func(v *ssa.Value, o int) {
			if old, ok := org[v]; !ok || old != o {
				org[v] = o
				changed = true
			}
		}
		for _, b := range f.Blocks[1:] {
			for i, p := range b.Params {
				if p.Type.Kind != ssa.KList {
					continue
				}
				o := top
				for _, e := range b.Preds {
					o = meet(o, get(e.Args[i]))
				}
				if o != top {
					set(p, o)
				}
			}
		}
		for _, b := range f.Blocks {
			for _, v := range b.Instrs {
				switch {
				case v.Op == ssa.OpReuse:
					if o := get(v.Args[0]); o != top {
						set(v, o)
					}
				case v.Op == ssa.OpCall && v.Aux == "list.set":
					if o := get(v.Args[0]); o != top {
						set(v, o)
					}
				case v.Op == ssa.OpField && v.Args[0].Op == ssa.OpCall && v.Type.Kind == ssa.KList:
					c := v.Args[0]
					callee := byName[c.Aux.(string)]
					k := v.Aux.(int)
					if callee == nil || k < 1 || k > len(callee.VarParams) {
						set(v, -1)
						continue
					}
					if o := get(c.Args[callee.VarParams[k-1]]); o != top {
						set(v, o)
					}
				case v.Type != nil && v.Type.Kind == ssa.KList:
					set(v, -1)
				}
			}
		}
	}
	return org
}

// goReject returns why f cannot be a Go mode function ("" when it can,
// as far as its own body goes), and the Xo functions it calls.
func (g *gen) goReject(f *ssa.Func) (string, []string) {
	if f.Name == "main" || f.Indirect || f.Entry || len(f.Captures) > 0 {
		return "not a plain function", nil
	}
	if strings.ContainsAny(f.Name, ".[") {
		return "a method, a generic instance, or a function of another module", nil
	}
	sig, err := g.goSigOf(f)
	if err != nil {
		return err.Error(), nil
	}
	if len(f.Results) != 1+len(f.VarParams) {
		return "unexpected results", nil
	}
	isVar := map[int]bool{}
	for _, i := range f.VarParams {
		if f.Params()[i].Type.Kind != ssa.KList {
			return "a `var` parameter that is not a List", nil
		}
		isVar[i] = true
	}
	org := goOrigins(f, g.byName)
	var deps []string
	for _, b := range f.Blocks {
		for _, v := range b.Instrs {
			if v.Type != nil && !goValue(v.Type) {
				return fmt.Sprintf("a value of type `%s`", v.Type.Name), nil
			}
			switch v.Op {
			case ssa.OpConst:
				if _, ok := v.Aux.(string); ok {
					return "a string", nil
				}
			case ssa.OpAdd, ssa.OpSub, ssa.OpMul, ssa.OpDiv, ssa.OpRem, ssa.OpNeg, ssa.OpAnd, ssa.OpOr,
				ssa.OpXor, ssa.OpNot, ssa.OpShl, ssa.OpShr, ssa.OpEq, ssa.OpNe, ssa.OpLt, ssa.OpLe, ssa.OpGt,
				ssa.OpGe, ssa.OpIntToFloat, ssa.OpFloatToInt, ssa.OpStruct, ssa.OpTuple, ssa.OpVariant,
				ssa.OpField, ssa.OpSetField, ssa.OpTag, ssa.OpPayload, ssa.OpJump, ssa.OpBranch, ssa.OpUnreachable:
			case ssa.OpRetain, ssa.OpRelease:
				if v.Args[0].Type.Heap() {
					return "reference counting", nil
				}
			case ssa.OpReuse:
				if o := org[v.Args[0]]; o < 0 || !isVar[o] {
					return "a List made unique that is not a `var` parameter", nil
				}
			case ssa.OpReturn:
				for k, a := range v.Args[1:] {
					if org[a] != f.VarParams[k] {
						return "a `var` List parameter that is not returned as it came", nil
					}
				}
			case ssa.OpCall:
				name := v.Aux.(string)
				if callee := g.byName[name]; callee != nil {
					deps = append(deps, name)
					continue
				}
				if !goMethods[name] {
					return fmt.Sprintf("a call of `%s`", name), nil
				}
				if name == "list.set" {
					o := org[v.Args[0]]
					if o < 0 || !isVar[o] {
						return "a write to a List that is not a `var` parameter", nil
					}
					g.goWrites[f.Name] = append(g.goWrites[f.Name], o)
				}
			default:
				return fmt.Sprintf("op `%s`", v.Op), nil
			}
		}
	}
	_ = sig
	return "", deps
}

// hasLoop reports whether f's control flow has a cycle.
func hasLoop(f *ssa.Func) bool {
	state := map[*ssa.Block]int{}
	var dfs func(b *ssa.Block) bool
	dfs = func(b *ssa.Block) bool {
		state[b] = 1
		if t := b.Term(); t != nil {
			for _, e := range t.Succs {
				switch state[e.To] {
				case 0:
					if dfs(e.To) {
						return true
					}
				case 1:
					return true
				}
			}
		}
		state[b] = 2
		return false
	}
	return dfs(f.Blocks[0])
}

// GenerateGo emits the eligible functions of m (after InsertRC) as Go
// assembler. A function the analysis accepts but instruction selection
// or the translation cannot handle is rejected with its reason, and so
// are the functions that call it.
func GenerateGo(m *ssa.Module, opts GoOptions) (prog *GoProgram, err error) {
	if !m.RC {
		return nil, &Error{"the module has no reference counting (run InsertRC)"}
	}
	if !opts.NoInline && inline(m) {
		if err := m.Verify(); err != nil {
			return nil, &InlineError{err}
		}
	}
	g := &gen{m: m, os: Linux, byName: map[string]*ssa.Func{}, layouts: map[*ssa.Type][]bool{},
		strs: map[string]string{}, locs: map[string]string{}, helped: map[string]string{},
		goMode: true, goFuncs: map[string]*GoFunc{}, goPosIx: map[string]int{}, goSigs: map[string]*goSig{},
		goWrites: map[string][]int{}}
	for _, f := range m.Funcs {
		g.byName[f.Name] = f
	}
	prog = &GoProgram{Funcs: map[string]*GoFunc{}, Rejected: map[string]string{}}
	deps := map[string][]string{}
	for _, f := range m.Funcs {
		why, ds := g.goReject(f)
		if why != "" {
			prog.Rejected[f.Name] = why
			continue
		}
		deps[f.Name] = ds
		g.goSigs[f.Name], _ = g.goSigOf(f)
	}
	propagate := func() {
		for changed := true; changed; {
			changed = false
			for _, f := range m.Funcs {
				if _, bad := prog.Rejected[f.Name]; bad {
					continue
				}
				for _, d := range deps[f.Name] {
					if _, bad := prog.Rejected[d]; bad {
						prog.Rejected[f.Name] = "calls `" + d + "`, which is not eligible"
						changed = true
						break
					}
				}
			}
		}
	}
	propagate()
	texts := map[string]string{}
	for {
		g.goFuncs = map[string]*GoFunc{}
		var order []*ssa.Func
		for _, f := range m.Funcs {
			if _, bad := prog.Rejected[f.Name]; bad {
				continue
			}
			if opts.All || hasLoop(f) {
				g.goFuncs[f.Name] = &GoFunc{Name: f.Name, Sym: GoSym(f.Name), Wrap: true}
			}
		}
		// the functions the selected ones call are emitted too
		for changed := true; changed; {
			changed = false
			for name := range g.goFuncs {
				for _, d := range deps[name] {
					if g.goFuncs[d] == nil {
						g.goFuncs[d] = &GoFunc{Name: d, Sym: GoSym(d)}
						changed = true
					}
				}
			}
		}
		for _, f := range m.Funcs {
			if g.goFuncs[f.Name] != nil {
				order = append(order, f)
			}
		}
		failed := false
		g.goPos, g.goPosIx = nil, map[string]int{}
		for _, f := range order {
			text, err := g.goFunction(f)
			if err != nil {
				prog.Rejected[f.Name] = err.Error()
				failed = true
				continue
			}
			texts[f.Name] = text
		}
		if !failed {
			var b strings.Builder
			for _, f := range order {
				b.WriteString(texts[f.Name])
				prog.Funcs[f.Name] = g.goFuncs[f.Name]
			}
			prog.Positions = g.goPos
			if b.Len() > 0 {
				prog.Asm = []byte(goAsmHeader + b.String())
			}
			return prog, nil
		}
		propagate()
	}
}

const goAsmHeader = `// Code generated by xo build --hybrid (internal/arm64, Go mode, decision 0084). DO NOT EDIT.

//go:build !xo_pure

#include "textflag.h"
#include "funcdata.h"

`

// goFunction emits one function in Go mode.
func (g *gen) goFunction(f *ssa.Func) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*Error)
			if !ok {
				panic(r)
			}
			text, err = "", e
		}
	}()
	g.text.Reset()
	g.function(f)
	return g.text.String(), nil
}

// fpLoc is the location of an argument slot in the callee.
func fpLoc(sl goSlot) mloc {
	l := mloc{reg: -1, base: "FP", name: sl.name, off: sl.off}
	if sl.size == 1 {
		l.size = 1
	}
	return l
}

// spLoc is the location of an argument slot of a call (the callee's
// frame starts at 8(RSP), past the saved link register).
func spLoc(sl goSlot) mloc {
	l := mloc{reg: -1, base: "sp", off: 8 + sl.off}
	if sl.size == 1 {
		l.size = 1
	}
	return l
}

// hdr is the offset of a List's first element from its word.
func (s *fstate) hdr() int {
	if s.g.goMode {
		return 0
	}
	return listHdr
}

func (s *fstate) goEntry(b *ssa.Block) {
	sig := s.g.goSigs[s.f.Name]
	in := &inst{kind: iEntry}
	for i, p := range b.Params {
		ws := s.newVregs(s.g.layout(p.Type))
		s.vals[p] = ws
		sl := sig.params[i]
		if p.Type.Kind == ssa.KList {
			n := s.newVreg(false)
			s.goLens[ws[0]] = n
			s.m().lens[ws[0]] = n
			in.defs = append(in.defs, ws[0], n)
			in.gargs = append(in.gargs, fpLoc(sl[0]), fpLoc(sl[1]))
			continue
		}
		for k, w := range ws {
			in.defs = append(in.defs, w)
			in.gargs = append(in.gargs, fpLoc(sl[k]))
		}
	}
	s.add(in)
}

func (s *fstate) goReturn(v *ssa.Value) {
	sig := s.g.goSigs[s.f.Name]
	in := &inst{kind: iRet}
	if len(v.Args) > 0 {
		for k, w := range s.words(v.Args[0]) {
			in.srcs = append(in.srcs, rsrc(w))
			in.gargs = append(in.gargs, fpLoc(sig.res[k]))
			if w != 0 {
				in.uses = append(in.uses, w)
			}
		}
	}
	s.add(in)
}

func (s *fstate) goReuse(v *ssa.Value) {
	// the wrapper made the `var` parameter unique (goReject checked it is one)
	s.set(v, s.words(v.Args[0])...)
}

func (s *fstate) goCall(v *ssa.Value, callee *ssa.Func) {
	g := s.g
	if g.goFuncs[callee.Name] == nil {
		fail("a call of %s, which is not emitted", callee.Name)
	}
	sig := g.goSigs[callee.Name]
	in := &inst{kind: iCall, sym: g.fname(callee.Name)}
	arg := func(w vreg, l mloc) {
		in.srcs = append(in.srcs, rsrc(w))
		in.gargs = append(in.gargs, l)
		if w != 0 {
			in.uses = append(in.uses, w)
		}
	}
	for i, a := range v.Args {
		sl := sig.params[i]
		ws := s.words(a)
		if a.Type.Kind == ssa.KList {
			l := s.real(ws[0])
			n := s.listLen(l)
			arg(l, spLoc(sl[0]))
			arg(n, spLoc(sl[1]))
			arg(n, spLoc(goSlot{off: sl[1].off + 8, size: 8})) // the capacity: the callee cannot grow it
			continue
		}
		for k, w := range ws {
			arg(w, spLoc(sl[k]))
		}
	}
	in.defs = s.newVregs(g.layout(callee.Results[0]))
	for k := range in.defs {
		in.gres = append(in.gres, spLoc(sig.res[k]))
	}
	s.add(in)
	s.clobber()
	out := append([]vreg(nil), in.defs...)
	for _, pi := range callee.VarParams {
		// the callee wrote the List in place
		out = append(out, s.words(v.Args[pi])...)
	}
	s.set(v, out...)
}

func (s *fstate) goEmitCall(b *strings.Builder, in *inst, fr *frameInfo) {
	var ms []move
	for k, a := range in.srcs {
		ms = append(ms, move{in.gargs[k], s.msrcOf(a, fr)})
	}
	for k, v := range in.saves {
		fmt.Fprintf(b, "\tstr %s, [sp, #%d]\n", regName(s.loc[v].reg, false), fr.spillBase+8*in.saveSlots[k])
	}
	s.parallel(b, ms, fr)
	fmt.Fprintf(b, "\tbl %s\n", in.sym)
	if in.noret {
		return
	}
	for k, v := range in.saves {
		fmt.Fprintf(b, "\tldr %s, [sp, #%d]\n", regName(s.loc[v].reg, false), fr.spillBase+8*in.saveSlots[k])
	}
	var rs []move
	for k, d := range in.defs {
		if dl, ok := s.vloc(d, fr); ok {
			rs = append(rs, move{dl, msrc{kind: mLoc, l: in.gres[k]}})
		}
	}
	s.parallel(b, rs, fr)
}

// goFaultKind numbers the faults of xoHybridFault (std/rt HybridFault).
var goFaultKind = map[string]int64{
	"xo_fault_index": 1, "xo_fault_overflow": 2, "xo_fault_divzero": 3, "xo_fault_neg": 4,
	"xo_fault_shl": 5, "xo_fault_shift": 6, "xo_fault_float_int": 7, "xo_fault_abs": 8,
}

// goFaultStub is the side exit of a checked instruction: a call of
// xoHybridFault(kind, a, b, c, pos), which does not return.
func (s *fstate) goFaultStub(b *strings.Builder, in *inst, fr *frameInfo) {
	g := s.g
	s.nstub++
	l := g.local(fmt.Sprintf("f%d_fault%d", g.nfunc, s.nstub))
	fmt.Fprintf(b, "\tb.%s %s\n", in.fault.cond, l)
	fmt.Fprintf(&s.stubs, "%s:\n", l)
	kind := goFaultKind[in.fault.fn]
	args := in.fault.args
	if kind == 0 || len(args) == 0 || len(args) > 4 {
		fail("fault %s", in.fault.fn)
	}
	last := args[len(args)-1]
	ps, ok := strings.CutPrefix(last.sym, "@pos:")
	if last.kind != sSym || !ok {
		fail("fault %s without a position", in.fault.fn)
	}
	pos, _ := strconv.Atoi(ps)
	vals := args[:len(args)-1]
	ms := make([]move, len(vals))
	for k, a := range vals {
		ms[k] = move{mloc{reg: -1, base: "sp", off: 16 + 8*k}, s.msrcOf(a, fr)}
	}
	s.recoverOperand(&s.stubs, in, ms, fr)
	ms = append(ms, move{mloc{reg: -1, base: "sp", off: 8}, msrc{kind: mImm, imm: kind}})
	for k := len(vals); k < 3; k++ {
		ms = append(ms, move{mloc{reg: -1, base: "sp", off: 16 + 8*k}, msrc{kind: mZero}})
	}
	ms = append(ms, move{mloc{reg: -1, base: "sp", off: 40}, msrc{kind: mImm, imm: int64(pos)}})
	s.parallel(&s.stubs, ms, fr)
	s.stubs.WriteString("\tbl ·xoHybridFault(SB)\n\tbrk #1\n")
}

// goEmit emits the function as Go assembler: the frame is the outgoing
// argument area (8(RSP) upward) and the spill slots after it; Go's
// assembler adds the saved link register and the prologue.
func (s *fstate) goEmit() {
	g := s.g
	gf := g.goFuncs[s.f.Name]
	sig := g.goSigs[s.f.Name]
	if s.out || len(s.saved) > 0 {
		fail("a frame slot of the native convention")
	}
	fr := &frameInfo{}
	out, calls, xcalls := 0, false, false
	for _, b := range s.order {
		for _, in := range b.insts {
			switch in.kind {
			case iCall:
				calls = true
				xcalls = xcalls || !in.noret
				for _, l := range append(append([]mloc(nil), in.gargs...), in.gres...) {
					out = max(out, l.off-8+8)
				}
			case iSwitch:
				fail("a jump table")
			}
			if in.fault != nil {
				calls = true
				out = max(out, 40)
			}
		}
	}
	fr.outArgs = alignUp(out, 8)
	fr.outOff = 8 + fr.outArgs
	fr.spillBase = fr.outOff
	frame := fr.outArgs + 8*s.slots
	nosplit := !xcalls && frame <= 256
	b := &strings.Builder{}
	live := map[*mblock]bool{}
	var walk func(mb *mblock)
	walk = func(mb *mblock) {
		if live[mb] {
			return
		}
		live[mb] = true
		for _, t := range mb.succs {
			walk(s.fwd(t, fr))
			if ti := s.fwd(t, fr).insts; len(ti) == 1 && ti[0].kind == iCondBr {
				for _, t2 := range ti[0].targets {
					walk(s.fwd(t2, fr))
				}
			}
		}
	}
	walk(s.order[0])
	var emitted []*mblock
	for _, mb := range s.order {
		if live[mb] {
			emitted = append(emitted, mb)
		}
	}
	for bi, mb := range emitted {
		var next *mblock
		if bi+1 < len(emitted) {
			next = emitted[bi+1]
		}
		fmt.Fprintf(b, "%s:\n", s.label(mb))
		s.cur = mb
		for k, in := range mb.insts {
			s.emitInst(b, in, fr, next)
			if k == 0 && checkHead(mb) != nil {
				fmt.Fprintf(b, "%s_c:\n", s.label(mb))
			}
		}
	}
	body, err := goText(pairLoads(b.String()) + "\n" + s.stubs.String())
	if err != nil {
		fail("%v", err)
	}
	flags := "0"
	if nosplit {
		flags = "NOSPLIT"
	}
	fmt.Fprintf(&g.text, "// func %s\nTEXT %s, %s, $%d-%d\n", s.f.Name, g.fname(s.f.Name), flags, frame, sig.size)
	if calls && frame > 0 {
		g.text.WriteString("\tNO_LOCAL_POINTERS\n")
	}
	g.text.WriteString(body)
	g.text.WriteString("\n")
	gf.NoSplit, gf.Frame, gf.ArgSize, gf.NoEscape = nosplit, frame, sig.size, !xcalls
	seen := map[int]bool{}
	gf.Unshare = nil
	for _, i := range g.goWrites[s.f.Name] {
		if !seen[i] {
			seen[i] = true
			gf.Unshare = append(gf.Unshare, i)
		}
	}
	sort.Ints(gf.Unshare)
}
