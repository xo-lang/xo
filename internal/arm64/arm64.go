// Package arm64 is the native AArch64 backend (decision 0049): it lowers
// the shared SSA IR (internal/ssa, after reference counting, decision
// 0046) straight to assembly text, with no LLVM. internal/build
// assembles and links the text with the system C compiler together with
// the C runtime std/llrt/xo_rt.c (`xo build --backend=arm64`).
//
// Pipeline, per function:
//
//  1. Instruction selection (isel.go) turns SSA values into machine
//     instructions over virtual registers (MIR, mir.go). Every SSA value
//     is flattened into 64 bit words (layout.go): an Int, Bool, or List
//     is one general purpose word, a Float one floating point word, a Str
//     two (object, length), a struct or tuple the words of its fields, an
//     enum a tag word and the payload words. Struct and tuple
//     construction, field reads, and payload reads of general purpose
//     words are aliases of the operand's registers (values are
//     immutable), so they cost nothing. Block parameters become parallel
//     moves at the end of each predecessor (critical edges with
//     arguments get a block of their own).
//  2. Register allocation (regalloc.go): liveness by data flow, live
//     ranges with holes per virtual register, and a greedy first fit in
//     priority order (uses weighted by loop depth) against the ranges
//     already given to each physical register. Calls clobber the caller
//     saved registers (a one point range at each call), so values live
//     across a call land in callee saved registers or are spilled. A
//     spilled register gets the first stack slot no value live at the
//     same time holds (decision 0146) and is reloaded into a scratch
//     register at each use. A frame past the reach of sp relative
//     offsets is allocated again with x28 kept free to address it.
//  3. Emission (emit.go): frame (frame pointer kept, 16 byte aligned),
//     parallel moves resolved with cycle breaking, out of line fault
//     stubs, reference counting helpers (rc.go), literals.
//
// The calling convention is AAPCS64 for every call: integer words in x0
// to x7, floating point words in d0 to d7, the rest on the stack in 8
// byte slots (Xo to Xo calls only; the runtime takes at most four
// scalars), results in the same registers; result words past x7 or d7
// are stored by the callee in the caller's stack argument area (the
// same 8 byte slots, which the caller reserves). Function values are
// called with `blr x0` (closures.go). x18 is never touched (Apple
// reserves it), x16 and x17 (and x15, d29 to d31) are scratch registers
// of the backend, x29 is the frame pointer. Darwin symbols get a leading
// underscore and Mach-O sections; Linux gets ELF sections.
package arm64

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ssa"
)

// OS is the object format and symbol convention of the output.
type OS string

const (
	Darwin OS = "darwin" // Mach-O: `_` symbol prefix, @PAGE relocations
	Linux  OS = "linux"  // ELF: no prefix, :lo12: relocations
)

// Options configure generation.
type Options struct {
	OS OS
	// NoInline turns off the inlining of small leaf functions.
	NoInline bool
	// Layout is the placement of code (align.go).
	Layout Layout
}

// InlineError is a module that no longer verifies after inlining (a bug
// of the inliner): the caller can build the module again and generate
// with NoInline.
type InlineError struct{ Err error }

func (e *InlineError) Error() string { return "arm64 backend: after inlining: " + e.Err.Error() }

// Program is the generated assembly of one Xo program.
type Program struct {
	Asm   []byte
	Funcs int
}

// Error is a construct the backend does not support.
type Error struct{ Msg string }

func (e *Error) Error() string { return "arm64 backend: " + e.Msg }

func fail(format string, args ...any) { panic(&Error{fmt.Sprintf(format, args...)}) }

// gen is the state of one program.
type gen struct {
	m       *ssa.Module
	os      OS
	byName  map[string]*ssa.Func
	layouts map[*ssa.Type][]bool
	strs    map[string]string
	strOrd  []string
	locs    map[string]string
	locOrd  []string
	helpers strings.Builder
	helped  map[string]string
	nhelp   int
	nfunc   int
	text    strings.Builder
	// unwinding (decision 0085): the call site tables, call site label
	// numbers, and whether the ELF personality reference is needed
	lsdas          strings.Builder
	ncs            int
	personalityRef bool
	// rt.sys_restorer was used: the rt_sigreturn trampoline is emitted
	// (decision 0206)
	restorerRef bool
	place       Layout
	// Go mode (goasm.go): functions as Go assembler for Go backend builds
	goMode   bool
	noPoll   bool               // GoOptions.NoPoll
	goFuncs  map[string]*GoFunc // the functions emitted, by name
	goPos    []string           // fault positions, indexed by the stubs
	goPosIx  map[string]int
	goSigs   map[string]*goSig
	goWrites map[string][]int // the `var` List parameters each function writes
	// large `var` parameters passed by reference (byref.go): the
	// parameters of each function, and the functions used as values
	byRef     map[*ssa.Func][]bool
	addrTaken map[string]bool
}

// InlineDone, when set, is called by Generate after inlining (and the
// verification after it), before emission: a timing hook for `xo asmdump`
// with XO_PHASE_TIMES=1.
var InlineDone func()

// Generate lowers a module (after InsertRC) to AArch64 assembly.
func Generate(m *ssa.Module, opts Options) (prog *Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*Error)
			if !ok {
				panic(r)
			}
			prog, err = nil, e
		}
	}()
	if !m.RC {
		return nil, &Error{"the module has no reference counting (run InsertRC)"}
	}
	if opts.OS == "" {
		opts.OS = Darwin
	}
	g := &gen{m: m, os: opts.OS, byName: map[string]*ssa.Func{}, layouts: map[*ssa.Type][]bool{},
		strs: map[string]string{}, locs: map[string]string{}, helped: map[string]string{}, place: opts.Layout}
	if !opts.NoInline && inline(m) {
		if err := m.Verify(); err != nil {
			return nil, &InlineError{err}
		}
	}
	if InlineDone != nil {
		InlineDone()
	}
	for _, f := range m.Funcs {
		g.byName[f.Name] = f
	}
	g.findAddrTaken()
	main := g.byName["main"]
	if main == nil {
		return nil, &Error{"no main"}
	}
	for _, f := range m.Funcs {
		g.function(f)
	}
	g.entry(main)
	var out strings.Builder
	out.WriteString("// generated by xo build --backend=arm64 (internal/arm64, from internal/ssa with reference counting)\n")
	if g.os == Linux {
		// a fixed file symbol: without one GNU ld names the object's
		// STT_FILE after gcc's temporary object (ccXXXXXX.o), and two
		// builds of one program differed (2026-10-12, gotcha
		// linux-arm64-file-symbol)
		out.WriteString("\t.file \"prog.s\"\n")
	}
	out.WriteString(g.section("text"))
	out.WriteString(g.text.String())
	out.WriteString(g.helpers.String())
	out.WriteString(g.lsdas.String())
	if g.restorerRef {
		// rt_sigaction's sa_restorer (decision 0206): rt_sigreturn, number 139
		fmt.Fprintf(&out, "\t.p2align 2\n%s:\n\tmov x8, #139\n\tsvc #0\n", g.local("xo_sigreturn"))
	}
	if g.personalityRef {
		// the personality routine through a data word, as ELF compilers
		// refer to it from the unwind tables
		out.WriteString("\t.hidden DW.ref.xo_personality\n\t.weak DW.ref.xo_personality\n" +
			"\t.section .data.DW.ref.xo_personality,\"awG\",@progbits,DW.ref.xo_personality,comdat\n" +
			"\t.p2align 3\n\t.type DW.ref.xo_personality, @object\n\t.size DW.ref.xo_personality, 8\n" +
			"DW.ref.xo_personality:\n\t.xword xo_personality\n")
	}
	g.data(&out)
	if g.os == Linux {
		out.WriteString("\t.section .note.GNU-stack,\"\",@progbits\n")
	}
	return &Program{Asm: []byte(out.String()), Funcs: len(m.Funcs)}, nil
}

// ---------------------------------------------------------------------------
// Target conventions

// sym is the assembler name of a C or global symbol.
func (g *gen) sym(name string) string {
	if g.os == Darwin {
		return "_" + name
	}
	return name
}

// local is an assembler local label.
func (g *gen) local(name string) string {
	if g.goMode {
		return "L" + name
	}
	if g.os == Darwin {
		return "L" + name
	}
	return ".L" + name
}

// fname is the symbol of Xo function n: letters and digits are kept, `_`
// is doubled, anything else is `_` and two hex digits.
func (g *gen) fname(n string) string {
	if g.goMode {
		return "·" + goSym(n) + "(SB)"
	}
	var b strings.Builder
	b.WriteString("xo_f_")
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
	return g.sym(b.String())
}

// addr returns the two instructions that put the address of symbol s in
// register r.
func (g *gen) addr(r, s string) string {
	if g.os == Darwin {
		return fmt.Sprintf("\tadrp %s, %s@PAGE\n\tadd %s, %s, %s@PAGEOFF\n", r, s, r, r, s)
	}
	return fmt.Sprintf("\tadrp %s, %s\n\tadd %s, %s, :lo12:%s\n", r, s, r, r, s)
}

func (g *gen) section(kind string) string {
	if g.os == Darwin {
		switch kind {
		case "text":
			return "\t.section __TEXT,__text,regular,pure_instructions\n"
		case "data":
			return "\t.section __DATA,__data\n"
		case "const":
			return "\t.section __TEXT,__const\n"
		case "cstring":
			return "\t.section __TEXT,__cstring,cstring_literals\n"
		}
	}
	switch kind {
	case "text":
		return "\t.text\n"
	case "data":
		return "\t.data\n"
	case "const":
		return "\t.section .rodata\n"
	}
	return "\t.section .rodata.str1.1,\"aMS\",@progbits,1\n"
}

// funcHeader starts a function symbol (local unless global).
func (g *gen) funcHeader(b *strings.Builder, s string, global bool) {
	fmt.Fprintf(b, "\t.p2align %d\n", max(2, g.place.Func))
	for range g.place.Pad {
		b.WriteString("\tnop\n")
	}
	if global {
		fmt.Fprintf(b, "\t.globl %s\n", s)
	}
	if g.os == Linux {
		fmt.Fprintf(b, "\t.type %s, %%function\n", s)
	}
	fmt.Fprintf(b, "%s:\n", s)
}

// immortal is the count of literals, never written: negative (so
// retains and releases leave the thread local fast path) with bit 62
// clear (so the shared path tells it from a shared count). Decision
// 0058; std/llrt/xo_rt.c, "Reference counts".
const immortal = int64(math.MinInt64)

// strObj returns the label of the immortal object of a string literal.
func (g *gen) strObj(s string) string {
	if l, ok := g.strs[s]; ok {
		return l
	}
	l := g.local(fmt.Sprintf("xo_str%d", len(g.strs)))
	g.strs[s] = l
	g.strOrd = append(g.strOrd, s)
	return l
}

// loc returns the label of the NUL terminated text of a fault position.
func (g *gen) loc(s string) string {
	if g.goMode {
		// an index into the position table of the Go package
		ix, ok := g.goPosIx[s]
		if !ok {
			ix = len(g.goPos)
			g.goPos = append(g.goPos, s)
			g.goPosIx[s] = ix
		}
		return fmt.Sprintf("@pos:%d", ix)
	}
	if l, ok := g.locs[s]; ok {
		return l
	}
	l := g.local(fmt.Sprintf("xo_loc%d", len(g.locs)))
	g.locs[s] = l
	g.locOrd = append(g.locOrd, s)
	return l
}

func (g *gen) data(out *strings.Builder) {
	if len(g.strOrd) > 0 {
		// read only: the immortal counts of literals are never written
		// (decision 0058), and a write would fault
		out.WriteString(g.section("const"))
		for _, s := range g.strOrd {
			fmt.Fprintf(out, "\t.p2align 3\n%s:\n\t.quad %d\n\t.quad %d\n", g.strs[s], immortal, len(s))
			if len(s) > 0 {
				fmt.Fprintf(out, "\t.ascii \"%s\"\n", asmBytes(s))
			}
		}
	}
	if len(g.locOrd) > 0 {
		out.WriteString(g.section("cstring"))
		for _, s := range g.locOrd {
			fmt.Fprintf(out, "%s:\n\t.asciz \"%s\"\n", g.locs[s], asmBytes(s))
		}
	}
}

// asmBytes escapes s for an .ascii directive.
func asmBytes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c >= 0x20 && c < 0x7f:
			b.WriteByte(c)
		default:
			b.WriteString("\\" + strconv.FormatInt(int64(c)+01000, 8)[1:])
		}
	}
	return b.String()
}

// entry emits xo_entry, which the C runtime's main calls: it calls the
// Xo main with zero words for its parameters.
func (g *gen) entry(main *ssa.Func) {
	b := &g.text
	g.funcHeader(b, g.sym("xo_entry"), true)
	b.WriteString("\t.cfi_startproc\n\tstp x29, x30, [sp, #-16]!\n\tmov x29, sp\n")
	b.WriteString("\t.cfi_def_cfa w29, 16\n\t.cfi_offset w30, -8\n\t.cfi_offset w29, -16\n")
	var ngp, nfp int
	for _, p := range main.Params() {
		for _, fp := range g.layout(p.Type) {
			if fp {
				if nfp < 8 {
					fmt.Fprintf(b, "\tfmov d%d, xzr\n", nfp)
				}
				nfp++
			} else {
				if ngp < 8 {
					fmt.Fprintf(b, "\tmov x%d, #0\n", ngp)
				}
				ngp++
			}
		}
	}
	if ngp > 8 || nfp > 8 {
		fail("main takes too many words")
	}
	fmt.Fprintf(b, "\tbl %s\n\tldp x29, x30, [sp], #16\n\tret\n\t.cfi_endproc\n\n", g.fname(main.Name))
}
