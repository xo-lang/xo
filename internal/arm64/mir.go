package arm64

// MIR: machine instructions over virtual registers.
//
// A virtual register (vreg) is an int; 0 is the zero register (xzr),
// which instruction selection uses for payload words no variant fills.
// Virtual registers need not be SSA: a few are defined on two paths
// (the result of a list push that may grow, the arms of `list.get`), and
// block parameters are defined by the parallel move of each predecessor.
//
// Plain instructions carry an assembly template: `$d0` is the first
// definition, `$u1` the second use; `$wd0` and `$wu1` the 32 bit names
// of integer registers; `$OUT` is the frame offset of the scratch slot
// runtime calls write a length into.

type vreg int

type ikind int

const (
	iPlain  ikind = iota
	iPmove        // defs[k] = srcs[k], all at once
	iCall         // call sym with srcs, defs are the result words
	iEntry        // defs are the function parameters, from the ABI locations
	iRet          // uses are the result words
	iJump         // to targets[0]
	iCondBr       // text sets flags (or cond is cbnz/cbz on uses[0]); targets[0] if cond, else targets[1]
	iTrap         // after a call that does not return
)

type srcKind int

const (
	sReg   srcKind = iota // a vreg (0: zero)
	sImm                  // an integer constant
	sSym                  // the address of a symbol
	sFrame                // the address of the $OUT slot
)

// src is an operand of a call, a fault stub, or a parallel move.
type src struct {
	kind srcKind
	v    vreg
	imm  int64
	sym  string
}

func rsrc(v vreg) src     { return src{kind: sReg, v: v} }
func isrc(n int64) src    { return src{kind: sImm, imm: n} }
func symsrc(s string) src { return src{kind: sSym, sym: s} }
func framesrc() src       { return src{kind: sFrame} }

// fault is the side exit of a checked instruction: when cond holds after
// it, the runtime function fn is called with args (it does not return).
type fault struct {
	cond string
	fn   string
	args []src
}

type inst struct {
	kind  ikind
	text  string
	defs  []vreg
	uses  []vreg // template uses first, then the vregs of srcs and fault args
	early bool   // the defs are written before the uses are dead (they must not share registers)
	// recover: how a fault stub recomputes an operand whose register the
	// result took ("add", "sub", "neg")
	recover string
	fault   *fault
	// iCall: the callee symbol and the arguments; iPmove: the sources
	sym  string
	srcs []src
	// iCondBr
	cond    string
	targets []*mblock
	// iCall: the call never returns (a fault)
	noret bool
	// konst: a plain instruction that makes a constant (no uses but
	// other constants)
	konst bool
}

type mblock struct {
	id    int
	insts []*inst
	cold  bool
	succs []*mblock
	preds []*mblock
	// filled by register allocation
	start, end int
	depth      int
}

func (b *mblock) term() *inst {
	if len(b.insts) == 0 {
		return nil
	}
	return b.insts[len(b.insts)-1]
}
