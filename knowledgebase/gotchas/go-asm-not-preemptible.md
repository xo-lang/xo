# A loop in Go assembly cannot be preempted

Status: Observed (2026-10-10, `xo build --hybrid`, decision 0084); fixed
by decision 0131.

Symptom: a hybrid function that loops for a long time holds its
goroutine's P until it returns. Measured on 2026-10-10
(`internal/gen/testdata/hybrid_preempt`, an 11.5 s Collatz loop in
assembly): the first GC's stop the world waited 11.6 s, so every
goroutine that allocated stalled with it (main's longest gap 11.56 s);
with GOMAXPROCS=1 no other goroutine ran until the loop returned.

Cause: the Go runtime never preempts assembly asynchronously
(`isAsyncSafePoint` refuses functions flagged as assembly: they have no
register maps). The only preemption point is the stack check in the
prologue of a function that is not NOSPLIT, and leaf hybrid functions are
NOSPLIT.

Fix (decision 0131): every back edge polls g.stackguard0, as `ldr x16,
[x28, #16]; tbnz x16, #63, stub` (Go's poison values have bit 63 set),
and the stub saves the live registers, calls `xoHybridYield`
(`runtime.Gosched`), and reloads them. Stop the world during the same
loop: 0.058 ms. Traps when touching this:

- Compare with bit 63, not with -1314 exactly: `add #1314; cbz` cost 24
  percent on a tight loop, the bit test 1 percent.
- A polling function must not be `//go:noescape`: the yield may move the
  goroutine's stack, and the List bases in spill slots (hidden from the
  GC by NO_LOCAL_POINTERS) must point into the heap.
- A frame of 0 bytes with a call makes Go's assembler treat the function
  as a leaf (the link register stays in x30 and the call clobbers it);
  polling functions get a frame of at least 8 bytes.
- Tests that time a stop the world must not run in parallel with the
  rest of their package, and should retry: a loaded host delayed one to
  38 ms by itself (TestHybridPreemption retries three times).
