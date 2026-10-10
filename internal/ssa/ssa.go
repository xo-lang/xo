// Package ssa is the shared, typed SSA intermediate representation that
// Xo backends lower from (decision 0045): the LLVM backend, the reference
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
// Reference counting: OpRetain, OpRelease, and OpReuse are inserted
// after building by InsertRC (rc.go, decision 0046): every heap value is
// owned once, consumed by an operand position that takes ownership (call
// arguments of owned parameters, constructors, edge arguments, ret,
// list updates) and borrowed elsewhere; a value is released on every
// path after its last use. See the comments on the ops.
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
	KFunc               // a function value (closure): Elems are the parameters, Ret the result (heap: its environment)
	KBox                // Box[T]: an immutable, reference counted heap copy of Elems[0]; breaks recursion in types
	// KHandle is a runtime object shared by copies (decision 0068): Task,
	// Scope, task.Group, Chan, Mutex, Atomic. One pointer to a reference
	// counted object of std/llrt, shared between threads from its
	// creation; Elems are its type arguments.
	KHandle
)

// Type is an SSA type. Types are interned per Module: compare pointers.
type Type struct {
	Kind     Kind
	Name     string     // printed name: `Int`, `Body`, `List[Op]`, `(Int, Bool)`
	Fields   []Field    // KStruct
	Variants []*Variant // KEnum
	Elems    []*Type    // KTuple elements; KList: Elems[0] is the element type; KFunc: parameters
	Ret      *Type      // KFunc: the result (a Result for a function value that can fail)
	// Coll marks the KStruct that represents a Map or Set (build_map.go):
	// its fields are not its value, so `==` on it is not structural
	Coll bool
	// Bits and Unsigned describe a sized integer type of kind KInt
	// (decision 0081): Int8 to UInt64, and Rune (32 bits, signed). Its
	// values are held in the 64 bit word as Int values are, sign or zero
	// extended, so every operation that cannot leave the range (moves,
	// comparisons of narrow values, `&`, `|`, `^`) is the Int operation;
	// the builder lowers the others to checked runtime calls. Bits is 0
	// for Int itself.
	Bits     int
	Unsigned bool
}

// Sized reports whether t is a sized integer type (not Int).
func (t *Type) Sized() bool { return t.Kind == KInt && t.Bits != 0 }

// U64 reports whether t is UInt64, whose values above the Int range are
// held as negative words: its order, division, and text are unsigned.
func (t *Type) U64() bool { return t.Kind == KInt && t.Bits == 64 && t.Unsigned }

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

// heap walks the types t contains; seen holds the types on the current
// path (not every type visited: two Int fields are not a recursion).
func (t *Type) heap(seen map[*Type]bool) bool {
	if seen[t] {
		// a recursive struct or enum is boxed
		return true
	}
	seen[t] = true
	defer delete(seen, t)
	switch t.Kind {
	case KStr, KList, KFunc, KBox, KHandle:
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

// HasFloat reports whether a value of t holds a Float, directly or
// through fields, variants, elements, lists, and boxes. `==` on such a
// value can be false for a value and itself (a NaN field, core 2.1), so
// equality helpers must not answer true for the same cell without
// comparing its contents (decisions/0053).
func (t *Type) HasFloat() bool {
	return t.hasFloat(map[*Type]bool{})
}

func (t *Type) hasFloat(seen map[*Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	switch t.Kind {
	case KFloat:
		return true
	case KStruct:
		for _, f := range t.Fields {
			if f.Type.hasFloat(seen) {
				return true
			}
		}
	case KEnum:
		for _, v := range t.Variants {
			for _, f := range v.Fields {
				if f.Type.hasFloat(seen) {
					return true
				}
			}
		}
	case KTuple, KList, KBox:
		for _, e := range t.Elems {
			if e.hasFloat(seen) {
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

	// Comparisons: two operands of one type, result Bool. OpEq and OpNe
	// also compare structs, tuples, enums, and lists structurally (the
	// operands are borrowed; backends generate a helper per type).
	OpEq
	OpNe
	OpLt
	OpLe
	OpGt
	OpGe

	// Conversions.
	OpIntToFloat
	OpFloatToInt // checked: faults on NaN and out of range
	// OpIntConv is Args[0], an integer, as the integer type of the result:
	// the same word. The builder emits it only where every value of the
	// source type is a value of the result type (a widening conversion,
	// decision 0081); other conversions are checked runtime calls.
	OpIntConv

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

	// Function values (closures). A closure is a code pointer and a
	// reference counted environment holding the captured values (by
	// value, at creation); its Func has Indirect set and the closure
	// itself as parameter 0.
	//   OpClosure: Aux is the Func name; Args are the captured values
	//              (consumed), matching Func.Captures. Result KFunc.
	//   OpCapture: Args[0] is the closure parameter of the function it
	//              is in; Aux the index into Func.Captures. A projection
	//              out of the environment (borrows it).
	//   OpCallValue: Args[0] is the KFunc value (borrowed), the rest the
	//              arguments (consumed: an Indirect function owns them).
	OpClosure
	OpCapture
	OpCallValue

	// Boxes (recursive types). A field of a struct or enum whose type
	// contains that struct or enum by value has type Box[T] (decision
	// 0048).
	//   OpBox:   Args[0] (consumed) copied into a new Box.
	//   OpUnbox: the value in the box Args[0]: a projection (borrows it).
	OpBox
	OpUnbox

	// OpFuncAddr is the address of the function named Aux, as an Int: an
	// entry function (Func.Entry) handed to the runtime, which calls it.
	OpFuncAddr

	// OpRebind is a function value with the code of the Indirect function
	// named Aux and the environment of the function value Args[0]
	// (consumed: the environment moves into the result). The two
	// functions' Captures must be the same types: the builder emits it
	// only for the method functions of an interface value, which share
	// one environment holding the concrete value (build_iface.go).
	OpRebind

	// Calls. Aux is the callee name (string). A user function is called
	// by its Func.Name; runtime and prelude operations (list ops, string
	// building, printing, math) use dotted names such as `list.get`,
	// `str.concat`, `Stdio.println`, `Float.sqrt` (see RuntimeCalls).
	OpCall

	// OpFault stops the program with the message in Args[0] (Str) at Pos.
	// It is a terminator.
	OpFault

	// Reference counting (never emitted by the builder; inserted by
	// InsertRC, decision 0046). They apply to the heap leaves of a value
	// (a Str, a List, an enum with heap payload), never to a struct or
	// tuple as a whole (see rc.go):
	//   OpRetain  Args[0]: increment the count (dup); no result.
	//   OpRelease Args[0]: decrement, freeing at zero and releasing the
	//             children (drop); no result.
	//   OpDrop    Args[0]: release only the children of a value whose
	//             cell is reused (Perceus "drop specialization"); not
	//             emitted yet.
	//   OpReuse   Args[0]: a List the next instruction updates (list.set,
	//             list.push, list.pop). It consumes Args[0] and yields a
	//             uniquely owned List with the same contents: the same
	//             cell when its count is 1 (the update is then in place),
	//             otherwise a copy (and Args[0] is released). InsertRC
	//             emits it only where uniqueness is not proven statically.
	//   OpShare   Args[0]: mark the value shared between threads
	//             (decision 0058): every thread local count it reaches
	//             becomes atomic. Borrows; no result. Emitted where a
	//             value crosses to another task (build_conc.go) and by
	//             InsertShareProbes (tests).
	OpRetain
	OpRelease
	OpDrop
	OpReuse
	OpShare

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
	OpIntToFloat: "itof", OpFloatToInt: "ftoi", OpIntConv: "iconv",
	OpStruct: "struct", OpTuple: "tuple", OpVariant: "variant", OpField: "field",
	OpSetField: "setfield", OpTag: "tag", OpPayload: "payload", OpCap: "cap", OpUndef: "undef",
	OpCall: "call", OpFault: "fault", OpClosure: "closure", OpCapture: "capture", OpCallValue: "callv", OpBox: "box", OpUnbox: "unbox", OpFuncAddr: "funcaddr", OpRebind: "rebind",
	OpRetain: "retain", OpRelease: "release", OpDrop: "drop", OpReuse: "reuse", OpShare: "share",
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
	// Pad is where a fault raised by this value lands (decision 0085,
	// landing.go): the `rt.unwinding` or `rt.landing` call whose branch
	// goes to the cleanup (its first successor) when the value faults.
	// Set on calls and on operations that can fault; nil when a fault
	// leaves the function without cleanup.
	Pad *Value
}

// Wrap is the Aux of an Int add, sub, or mul without Pos whose overflow
// wraps by design (hash mixing). Other unchecked arithmetic cannot
// overflow, and the LLVM backend tells LLVM so (nsw); a wrapping one
// marked that way is poison when it overflows, which the optimizer may
// fold into wrong code (gotchas/llvm-nsw-hash-overflow.md).
const Wrap = "wrap"

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
	// Borrowed marks, per parameter, the ones the callee only reads (set
	// by InsertRC's borrow inference): the caller keeps ownership, and
	// the callee never releases them. Nil before InsertRC.
	Borrowed []bool
	// Indirect marks a function called through function values (a
	// closure body or the wrapper of a named function): parameter 0 is
	// the closure (borrowed), the others are owned, since callers do not
	// know the callee. Captures are the types of its environment.
	Indirect bool
	Captures []*Type
	// Entry marks a function the runtime calls (decision 0068: a task's
	// body, the release of a list the runtime holds): its parameters are
	// owned, its arguments and result are scalars or pointers (the C
	// calling convention), and its address is taken with OpFuncAddr.
	Entry bool
	// UnwindExit is the block that ends the function when it unwinds
	// (cancellation or a fault, decision 0085): it hands a fault on to
	// the caller (`rt.fault_continue`) or returns zero values. Nil when
	// nothing in the function unwinds.
	UnwindExit *Block
	// Catches marks a function whose unwind exit returns even on a fault
	// (the fault stays pending, as cancellation does): the closure given
	// to Mutex.with with a `var` parameter, which hands back the value it
	// was given so the mutex keeps it. Its caller checks for unwinding.
	Catches bool
	// landRest are the blocks addLandings split off after an operation
	// (merged back when its check goes, landing.go)
	landRest  map[*Block]bool
	nextValue int
	nextBlock int
}

// Params returns the function's parameters.
func (f *Func) Params() []*Value { return f.Blocks[0].Params }

// Module is a lowered program.
type Module struct {
	Funcs []*Func
	// RC is set by InsertRC: the functions carry explicit retain,
	// release, and reuse instructions, and Verify checks their balance.
	RC bool
	// FaultUnwind is set when faults unwind (decision 0085, landing.go):
	// faults are raised by `rt.fault_raise`, and the values that can
	// fault carry their landing (Value.Pad).
	FaultUnwind bool
	// TZ is the zone data std/time needs (build_time.go).
	TZ     TZUse
	faults map[*Func]bool // mayFault while Verify runs
	types  map[string]*Type
	order  []*Type // named struct and enum types, in creation order
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

// Float32 returns the interned type Float32: a KFloat whose values are
// doubles holding float32 values (Bits 32). Arithmetic is the double
// operation rounded to float32 (`rt.f32_round`), which is the correctly
// rounded float32 result for + - * / (a double has more than twice the
// precision); text and comparisons are the double's, as on the Go
// backend (std/rt's FloatStr(float64(x))).
func (m *Module) Float32() *Type {
	if t := m.types["Float32"]; t != nil {
		return t
	}
	t := &Type{Kind: KFloat, Name: "Float32", Bits: 32}
	m.types["Float32"] = t
	return t
}

// F32 reports whether t is Float32.
func (t *Type) F32() bool { return t.Kind == KFloat && t.Bits == 32 }

// sizedInts are the sized integer types: bits and signedness.
var sizedInts = map[string]struct {
	bits     int
	unsigned bool
}{
	"Int8": {8, false}, "Int16": {16, false}, "Int32": {32, false},
	"UInt8": {8, true}, "UInt16": {16, true}, "UInt32": {32, true}, "UInt64": {64, true},
	"Rune": {32, false},
}

// IntType returns the interned sized integer type named name (Int8 to
// UInt64, or Rune), or nil.
func (m *Module) IntType(name string) *Type {
	if t := m.types[name]; t != nil {
		return t
	}
	si, ok := sizedInts[name]
	if !ok {
		return nil
	}
	t := &Type{Kind: KInt, Name: name, Bits: si.bits, Unsigned: si.unsigned}
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

// Box returns the interned Box[elem] type.
func (m *Module) Box(elem *Type) *Type {
	name := "Box[" + elem.Name + "]"
	if t := m.types[name]; t != nil {
		return t
	}
	t := &Type{Kind: KBox, Name: name, Elems: []*Type{elem}}
	m.types[name] = t
	return t
}

// Handle returns the interned handle type named name (decision 0068).
func (m *Module) Handle(name string, elems ...*Type) *Type {
	if t := m.types[name]; t != nil {
		return t
	}
	t := &Type{Kind: KHandle, Name: name, Elems: elems}
	m.types[name] = t
	return t
}

// FuncType returns the interned type of function values taking params
// and returning ret.
func (m *Module) FuncType(params []*Type, ret *Type) *Type {
	var names []string
	for _, p := range params {
		names = append(names, p.Name)
	}
	name := "fn(" + strings.Join(names, ", ") + ") -> " + ret.Name
	if t := m.types[name]; t != nil {
		return t
	}
	t := &Type{Kind: KFunc, Name: name, Elems: params, Ret: ret}
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
	"list.new":         "() -> List[T]: an empty list",
	"list.len":         "(List[T]) -> Int",
	"list.get":         "(List[T], Int) -> Option[T]",
	"list.index":       "(List[T], Int) -> T: faults when out of range (Pos)",
	"list.set":         "(List[T], Int, T) -> List[T]: faults when out of range; in place when unique",
	"list.push":        "(List[T], T) -> List[T]",
	"list.pop":         "(List[T]) -> (List[T], Option[T])",
	"str.concat":       "(Str, Str) -> Str: consumes the left operand (appended in place when unique)",
	"str.eq":           "(Str, Str) -> Bool",
	"str.cmp":          "(Str, Str) -> Int: negative, zero, or positive",
	"str.concat_int":   "(Str, Int) -> Str: str.concat with the Int's text, no temporary (interpolation)",
	"str.concat_float": "(Str, Float) -> Str: str.concat with the Float's text, no temporary",
	"str.of_int":       "(Int) -> Str",
	"str.of_float":     "(Float) -> Str",
	"str.of_bool":      "(Bool) -> Str",
	"Stdio.print":      "(Stdio, Str) -> ()",
	"Stdio.println":    "(Stdio, Str) -> ()",
	"Stdio.eprint":     "(Stdio, Str) -> ()",
	"Stdio.eprintln":   "(Stdio, Str) -> ()",
	"Int.min":          "(Int, Int) -> Int",
	"Int.max":          "(Int, Int) -> Int",
	"Int.abs":          "(Int) -> Int: faults on the minimum Int",
	"Float.min":        "(Float, Float) -> Float",
	"Float.max":        "(Float, Float) -> Float",
	"Float.sqrt":       "(Float) -> Float (also floor, ceil, trunc, round, abs)",
	"Str.len":          "(Str) -> Int",
	"bytes.index":      "(Bytes, Int) -> UInt8: the byte at the index; faults when out of range (Pos)",
	"rt.landing": "() -> Bool: marks a check that only a fault takes (decision 0085, landing.go): " +
		"backends emit no call and branch to the second successor; the first runs when the value whose Pad it is faults",
	"rt.*": "a call of the C runtime function xo_* (std/llrt, decision 0068): every operand is an Int, " +
		"Bool, List, or handle (one word) or a Str (its bytes and length); the result is unit or one word",
}
