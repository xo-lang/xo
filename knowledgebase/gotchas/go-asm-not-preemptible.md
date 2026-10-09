# A loop in Go assembly cannot be preempted

Status: Anticipated (2026-10-09, `xo build --hybrid`, decision 0084)

Symptom (expected): a hybrid function that loops for a long time (sieve
`mark` over 10M elements runs about 15 ms per call) delays every stop
the world phase of the GC and every preemption of its goroutine until it
returns; other goroutines needing a GC see the pause as latency.

Cause: the Go runtime never preempts assembly asynchronously
(`isAsyncSafePoint` refuses functions flagged as assembly: they have no
register maps). The only preemption point is the stack check in the
prologue of a function that is not NOSPLIT, and leaf hybrid functions are
NOSPLIT.

Fix (not done): poll at loop back edges, as the Go compiler does for
loops without calls: load `g.stackguard0` (`16(R28)`), compare with
`stackPreempt`, and on a hit call a Go function that yields
(`runtime.Gosched` through a small wrapper), saving live registers
around it; a polling function must not be `//go:noescape` (its slices
must not move). Until then keep `--hybrid` off for servers whose hot
loops run long between returns.
