package rt

import (
	"math"
	"runtime"
)

// HybridFault raises the fault of a function that `xo build --hybrid`
// compiled to Go assembler (internal/arm64 Go mode, decision 0084): the
// assembly calls its package's xoHybridFault with a kind, up to three
// operands, and the index of the position, which calls this. Each kind
// raises what the Go backend's own check raises at the same operation.
func HybridFault(kind, a, b, c int64, at string) {
	switch kind {
	case 1: // index a out of range for length b
		panic(&arithFault{a, b, 'i', at})
	case 2: // a <op b> c overflowed
		panic(&arithFault{a, c, byte(b), at})
	case 3: // a <op b> 0
		panic(&arithFault{a, 0, byte(b), at})
	case 4:
		Faultf(at, "integer overflow: -(%s) does not fit Int", istr(a))
	case 5:
		Faultf(at, "integer overflow: %s << %s does not fit Int", istr(a), istr(b))
	case 6:
		Faultf(at, "shift amount %s out of range for Int", istr(a))
	case 7: // a holds the bits of the Float
		FloatToInt[int64](math.Float64frombits(uint64(a)), at)
	case 8:
		Faultf(at, "integer overflow: abs(%d) does not fit Int", a)
	}
	Faultf(at, "internal: unknown fault %d of a hybrid function", kind)
}

// HybridYield is called by an assembly loop of a hybrid build when its
// preemption poll sees Go's preemption request in g.stackguard0
// (decision 0131): the goroutine yields to the scheduler, where a stop
// the world, a stack scan, or another goroutine gets its turn, and the
// loop resumes where it was.
func HybridYield() {
	runtime.Gosched()
}
