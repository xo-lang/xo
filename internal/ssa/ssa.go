// Package ssa is the shared, typed SSA intermediate representation that
// Xo backends lower from (decision 0044): the LLVM backend, the reference
// counting pass with Perceus reuse, and a future native arm64 backend.
//
// Shape, in short:
//   - a Module holds Funcs and the Types they use (interned, so two types
//     are equal when their pointers are);
//   - a Func is a list of Blocks; Blocks[0] is the entry, whose block
//     parameters are the function's parameters;
//   - a Block has parameters (phi nodes written as block arguments, as in
//     MLIR and Cranelift), a list of instructions, and exactly one
//     terminator as its last instruction;
//   - every instruction is a *Value: an Op, a result Type, operands
//     (Args), and an Aux payload (constant, field index, callee name);
//   - a terminator carries Edges: a target block and the arguments for
//     its parameters.
//
// Values are immutable: Xo has value semantics, so a struct field update
// is OpSetField (a new struct), and a list mutation is a call that
// returns the new list (`list.set`, `list.push`, `list.pop`). Mutable
// locals and `var` parameters are SSA renamed by the builder (`var`
// parameters come back as extra results). This is what Perceus needs:
// with explicit ownership, `list.set` on a uniquely owned list is done
// in place.
//
// Reference counting hooks: OpRetain, OpRelease, OpDrop, and OpReuse
// exist in the instruction set but the builder never emits them. The RC
// pass (next step) inserts them after building: a value of a type with
// Type.Heap() true is owned by its definition, consumed by an operand
// position that takes ownership (call arguments, constructors, edge
// arguments, ret), and borrowed elsewhere. See the comments on the ops.
package ssa

import (
	"fmt"
	"strings"
)

// Kind classifies a Type.
type Kind int

const (
	KUnit   Kind = iota // `()`, zero size
	KInt                // 64 bit signed, checked arithmetic
	KFloat              // IEEE 754 double
	KBool               // 1 bit
	KStr                // immutable UTF-8 string (heap)
	KStruct             // named struct: Fields
	KEnum               // named enum: Variants (Option and Result too)
	KTuple              // anonymous product: Elems
	KList               // List[Elem] (heap, value semantics)
	KCap                // a capability such as Os or Stdio, zero size
)

// Type is an SSA type. Types are interned per Module: compare pointers.
type Type struct {
	Kind     Kind
	Name     string     // printed name: `Int`, `Body`, `List[Op]`, `(Int, Bool)`
	Fields   []Field    // KStruct
	Variants []*Variant // KEnum
	Elems    []*Type    // KTuple elements; KList: Elems[0] is the element type
}

// Field is a named struct field or variant field.
type Field struct {
	Name string
	Type *Type
}

// Variant is one enum variant; its tag is its index in Type.Variants.
type Variant struct {
	Name   string
	Fields []Field
}

func (t *Type) String() string { return t.Name }

// Heap reports whether values of t hold heap memory and so take part in
// reference counting: the RC pass retains and releases only these.
func (t *Type) Heap() bool {
	return t.heap(map[*Type]bool{})
}

func (t *Type) heap(seen map[*Type]bool) bool {
	if seen[t] {
		// a recursive struct or enum is boxed
		return true
	}
	seen[t] = true
	switch t.Kind {
	case KStr, KList:
		return true
	case KStruct:
		for _, f := range t.Fields {
			if f.Type.heap(seen) {
				return true
			}
		}
	case KEnum:
		for _, v := range t.Variants {
			for _, f := range v.Fields {
				if f.Type.heap(seen) {
					return true
				}
			}
		}
	case KTuple:
		for _, e := range t.Elems {
			if e.heap(seen) {
				return true
			}
		}
	}
	return false
}

// FieldIndex returns the index of the named struct field, or -1.
func (t *Type) FieldIndex(name string) int {
	for i, f := range t.Fields {
		if f.Name == name {
			return i
		}
	}
	return -1
}

// VariantIndex returns the tag of the named variant, or -1.
func (t *Type) VariantIndex(name string) int {
	for i, v := range t.Variants {
		if v.Name == name {
			return i
		}
	}
	return -1
}

// Op is an instruction opcode.
type Op int

const (
	OpInvalid Op = iota
	OpParam      // block parameter (a phi); not in Block.Instrs

	// Constants: Aux is int64, float64, bool, or string.
	OpConst

	// Arithmetic on Int (checked: faults on overflow and division by
	// zero, at Pos) or Float, by the operand type. Two operands.
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpRem
	OpNeg // one operand; checked on Int
	OpAnd // bitwise on Int, logical (non short circuit) on Bool
	OpOr
	OpXor
	OpNot // `!` on Bool, `~` on Int
	OpShl // checked: faults on a negative or too large shift, or overflow
	OpShr

	// Comparisons: two operands of one type, result Bool.
	OpEq
	OpNe
	OpLt
	OpLe
	OpGt
	OpGe

	// Conversions.
	OpIntToFloat
	OpFloatToInt // checked: faults on NaN and out of range

	// Aggregates. Construction consumes its operands (ownership moves in).
	OpStruct   // Args are the fields in declaration order
	OpTuple    // Args are the elements
	OpVariant  // Aux is the tag (int); Args are the variant's fields
	OpField    // Args[0] is a struct or tuple; Aux is the field index (borrow)
	OpSetField // Args[0] with field Aux replaced by Args[1]: a new value
	OpTag      // the tag of an enum value, as Int
	OpPayload  // field Aux.(PayloadRef).Field of variant Aux.(PayloadRef).Tag of Args[0]
	OpCap      // a capability projection: Aux is the field name (`os.stdio`)

	// OpUndef is a placeholder value in code after a terminator (the value
	// of a `return` or `break` expression). It only appears while building:
	// unreachable blocks are pruned, and Verify rejects it.
	OpUndef

	// Calls. Aux is the callee name (string). A user function is called
	// by its Func.Name; runtime and prelude operations (list ops, string
	// building, printing, math) use dotted names such as `list.get`,
	// `str.concat`, `Stdio.println`, `Float.sqrt` (see RuntimeCalls).
	OpCall

	// OpFault stops the program with the message in Args[0] (Str) at Pos.
	// It is a terminator.
	OpFault

	// Reference counting (never emitted by the builder; inserted by the
	// RC pass, decision 0044):
	//   OpRetain  Args[0]: increment the count of a Heap() value (dup).
	//   OpRelease Args[0]: decrement, freeing at zero (drop).
	//   OpDrop    Args[0]: release only the children of a value whose
	//             cell is reused (Perceus "drop specialization").
	//   OpReuse   Args[0]: the memory token of a value that is dead after
	//             this point and unique at run time; a later OpStruct,
	//             OpVariant, or list call with Aux reuse token writes into
	//             it in place (Perceus reuse analysis).
	OpRetain
	OpRelease
	OpDrop
	OpReuse

	// Terminators.
	OpJump        // Succs[0]
	OpBranch      // Args[0] Bool: Succs[0] if true, Succs[1] if false
	OpReturn      // Args are the function's results
	OpUnreachable // proven unreachable by the checker
)

var opNames = [...]string{
	OpInvalid: "invalid", OpParam: "param", OpConst: "const",
	OpAdd: "add", OpSub: "sub", OpMul: "mul", OpDiv: "div", OpRem: "rem", OpNeg: "neg",
	OpAnd: "and", OpOr: "or", OpXor: "xor", OpNot: "not", OpShl: "shl", OpShr: "shr",
	OpEq: "eq", OpNe: "ne", OpLt: "lt", OpLe: "le", OpGt: "gt", OpGe: "ge",
	OpIntToFloat: "itof", OpFloatToInt: "ftoi",
	OpStruct: "struct", OpTuple: "tuple", OpVariant: "variant", OpField: "field",
	OpSetField: "setfield", OpTag: "tag", OpPayload: "payload", OpCap: "cap", OpUndef: "undef",
	OpCall: "call", OpFault: "fault",
	OpRetain: "retain", OpRelease: "release", OpDrop: "drop", OpReuse: "reuse",
	OpJump: "jump", OpBranch: "br", OpReturn: "ret", OpUnreachable: "unreachable",
}

func (o Op) String() string {
	if int(o) < len(opNames) && opNames[o] != "" {
		return opNames[o]
	}
	return fmt.Sprintf("op%d", int(o))
}

// IsTerminator reports whether o ends a block.
func (o Op) IsTerminator() bool {
	switch o {
	case OpJump, OpBranch, OpReturn, OpUnreachable, OpFault:
		return true
	}
	return false
}

// PayloadRef addresses a variant field for OpPayload.
type PayloadRef struct {
	Tag, Field int
}

// Value is an instruction or a block parameter.
type Value struct {
	ID    int
	Op    Op
	Type  *Type // nil for instructions without a result (terminators, print calls return unit)
	Args  []*Value
	Aux   any
	Block *Block
	Succs []*Edge // terminators
	Pos   string  // `file:line:col` for faults; empty when it cannot fault
}

// Name is the printed name of v: `v12`.
func (v *Value) Name() string { return fmt.Sprintf("v%d", v.ID) }

// Edge is a control flow edge with the arguments for the target's
// parameters.
type Edge struct {
	From *Block
	To   *Block
	Args []*Value
}

// Block is a basic block.
type Block struct {
	ID     int
	Params []*Value
	Instrs []*Value // the last one is the terminator
	Preds  []*Edge
	Func   *Func
}

// Name is the printed name of b: `b3`.
func (b *Block) Name() string { return fmt.Sprintf("b%d", b.ID) }

// Term returns the terminator of b, or nil.
func (b *Block) Term() *Value {
	if len(b.Instrs) == 0 {
		return nil
	}
	t := b.Instrs[len(b.Instrs)-1]
	if !t.Op.IsTerminator() {
		return nil
	}
	return t
}

// Func is a function. Its parameters are Blocks[0].Params.
type Func struct {
	Name    string
	Results []*Type // one result; a function with `var` parameters returns them after it
	Blocks  []*Block
	// VarParams lists, in order, the indexes of the parameters that are
	// `var` (returned after the result, updated).
	VarParams []int
	nextValue int
	nextBlock int
}

// Params returns the function's parameters.
func (f *Func) Params() []*Value { return f.Blocks[0].Params }

// Module is a lowered program.
type Module struct {
	Funcs []*Func
	types map[string]*Type
	order []*Type // named struct and enum types, in creation order
}

// NewModule returns an empty module with the builtin types.
func NewModule() *Module {
	return &Module{types: map[string]*Type{}}
}

// Builtin returns the builtin type of kind k (unit, Int, Float, Bool, Str).
func (m *Module) Builtin(k Kind) *Type {
	name := map[Kind]string{KUnit: "()", KInt: "Int", KFloat: "Float", KBool: "Bool", KStr: "Str"}[k]
	if t := m.types[name]; t != nil {
		return t
	}
	t := &Type{Kind: k, Name: name}
	m.types[name] = t
	return t
}

// Tuple returns the interned tuple type of elems (unit when empty).
func (m *Module) Tuple(elems ...*Type) *Type {
	if len(elems) == 0 {
		return m.Builtin(KUnit)
	}
	var names []string
	for _, e := range elems {
		names = append(names, e.Name)
	}
	name := "(" + strings.Join(names, ", ") + ")"
	if t := m.types[name]; t != nil {
		return t
	}
	t := &Type{Kind: KTuple, Name: name, Elems: elems}
	m.types[name] = t
	return t
}

// List returns the interned List[elem] type.
func (m *Module) List(elem *Type) *Type {
	name := "List[" + elem.Name + "]"
	if t := m.types[name]; t != nil {
		return t
	}
	t := &Type{Kind: KList, Name: name, Elems: []*Type{elem}}
	m.types[name] = t
	return t
}

// Named returns the interned named type, creating an empty one of kind
// k (filled by the caller) when it is new. created reports that.
func (m *Module) Named(name string, k Kind) (t *Type, created bool) {
	if t := m.types[name]; t != nil {
		return t, false
	}
	t = &Type{Kind: k, Name: name}
	m.types[name] = t
	if k == KStruct || k == KEnum {
		m.order = append(m.order, t)
	}
	return t, true
}

// Func returns the function named name, or nil.
func (m *Module) Func(name string) *Func {
	for _, f := range m.Funcs {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// NewFunc adds a function with an entry block whose parameters have the
// given types.
func (m *Module) NewFunc(name string, params []*Type, results []*Type) *Func {
	f := &Func{Name: name, Results: results}
	entry := f.NewBlock()
	for _, p := range params {
		entry.AddParam(p)
	}
	m.Funcs = append(m.Funcs, f)
	return f
}

// NewBlock appends an empty block.
func (f *Func) NewBlock() *Block {
	b := &Block{ID: f.nextBlock, Func: f}
	f.nextBlock++
	f.Blocks = append(f.Blocks, b)
	return b
}

func (f *Func) newValue(op Op, t *Type, args []*Value, aux any) *Value {
	v := &Value{ID: f.nextValue, Op: op, Type: t, Args: args, Aux: aux}
	f.nextValue++
	return v
}

// AddParam appends a block parameter of type t.
func (b *Block) AddParam(t *Type) *Value {
	v := b.Func.newValue(OpParam, t, nil, nil)
	v.Block = b
	b.Params = append(b.Params, v)
	return v
}

// Add appends an instruction to b and returns it.
func (b *Block) Add(op Op, t *Type, aux any, args ...*Value) *Value {
	v := b.Func.newValue(op, t, args, aux)
	v.Block = b
	b.Instrs = append(b.Instrs, v)
	return v
}

// Jump ends b with a jump to to.
func (b *Block) Jump(to *Block, args ...*Value) *Value {
	t := b.Add(OpJump, nil, nil)
	t.Succs = []*Edge{link(b, to, args)}
	return t
}

// Branch ends b with a conditional branch.
func (b *Block) Branch(cond *Value, yes, no *Block) *Value {
	t := b.Add(OpBranch, nil, nil, cond)
	t.Succs = []*Edge{link(b, yes, nil), link(b, no, nil)}
	return t
}

func link(from, to *Block, args []*Value) *Edge {
	e := &Edge{From: from, To: to, Args: args}
	to.Preds = append(to.Preds, e)
	return e
}

// RuntimeCalls documents the runtime and prelude operations the builder
// emits as OpCall, with their operand and result shapes. Backends map
// each name to a runtime function or inline code.
var RuntimeCalls = map[string]string{
	"list.new":       "() -> List[T]: an empty list",
	"list.len":       "(List[T]) -> Int",
	"list.get":       "(List[T], Int) -> Option[T]",
	"list.index":     "(List[T], Int) -> T: faults when out of range (Pos)",
	"list.set":       "(List[T], Int, T) -> List[T]: faults when out of range; in place when unique",
	"list.push":      "(List[T], T) -> List[T]",
	"list.pop":       "(List[T]) -> (List[T], Option[T])",
	"str.concat":     "(Str, Str) -> Str",
	"str.eq":         "(Str, Str) -> Bool",
	"str.cmp":        "(Str, Str) -> Int: negative, zero, or positive",
	"str.of_int":     "(Int) -> Str",
	"str.of_float":   "(Float) -> Str",
	"str.of_bool":    "(Bool) -> Str",
	"Stdio.print":    "(Stdio, Str) -> ()",
	"Stdio.println":  "(Stdio, Str) -> ()",
	"Stdio.eprint":   "(Stdio, Str) -> ()",
	"Stdio.eprintln": "(Stdio, Str) -> ()",
	"Int.min":        "(Int, Int) -> Int",
	"Int.max":        "(Int, Int) -> Int",
	"Int.abs":        "(Int) -> Int: faults on the minimum Int",
	"Float.min":      "(Float, Float) -> Float",
	"Float.max":      "(Float, Float) -> Float",
	"Float.sqrt":     "(Float) -> Float (also floor, ceil, trunc, round, abs)",
	"Str.len":        "(Str) -> Int",
}
