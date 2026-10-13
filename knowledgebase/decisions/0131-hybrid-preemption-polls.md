# 0131: Hybrid assembly loops poll for preemption at back edges

Status: Accepted, implemented (2026-10-10). Amends decision 0084 (its
open item: assembly could not be preempted). Implemented in
`internal/arm64/goasm.go` (`insertPolls`, `goEmitPoll`),
`internal/gen/hybrid.go` (`xoHybridYield`), `std/rt/hybrid.go`
(`HybridYield`). Evidence: `bench/results/hybrid-preemption-2026-10-10.md`.

## Context
The Go runtime preempts goroutines asynchronously with a signal, but only
at PCs it can describe: it refuses every function written in assembly
(`isAsyncSafePoint`). The only other preemption point is a call's
prologue stack check, and hybrid leaf functions are NOSPLIT. A hybrid
loop therefore kept its P until it returned: measured on 2026-10-10, an
11.5 s loop made a GC stop the world wait 11.6 s, stalling every
goroutine that allocated, and with GOMAXPROCS=1 no other goroutine ran.
Decision 0084 kept `--hybrid` an option partly for this reason.

## Decision
1. **Every loop back edge of a Go mode function polls** Go's preemption
   request: `ldr x16, [x28, #16]` (g.stackguard0) and `tbnz x16, #63` to
   a stub after the body. The runtime writes stackPreempt (-1314) there
   for sysmon's 10 ms time slice, a stop the world, and a stack scan; a
   stack bound is a user space address, so bit 63 is set only by Go's
   poison values (stackPreempt, stackFork). Two instructions, no flags,
   no register reserved.
2. **The stub** stores the registers live across the poll to their own
   slots (as around a call), calls the package's `xoHybridYield`
   (`rt.HybridYield`: `runtime.Gosched`), reloads, and jumps back. The
   loop resumes with its state in registers; nothing is returned to Go.
3. **A function that polls is not `//go:noescape`**: its Lists' arrays
   are then heap allocated (the heap does not move), because the yield
   may move the goroutine's stack while the assembly holds addresses in
   spill slots that NO_LOCAL_POINTERS hides from the GC. It stays NOSPLIT
   when it calls nothing else and its frame is at most 256 bytes; its
   frame is at least 8 bytes (a frame of 0 would make Go's assembler keep
   the link register in x30 across the call).
4. **Polling, not chunking.** Chunking the loop from Go (the wrapper
   calling the assembly on ranges) is cheaper where the chunks are large
   (sieve: 1.006x against 1.070x) but bounds latency only when every
   chunk's work is bounded, which inner `while` loops (Collatz, the
   interpreter's dispatch) are not; doing it in the compiler needs every
   loop resumable plus a budget counter per back edge, which costs what
   the poll costs (1.062x).
5. `build.Options.HybridNoPoll` / `arm64.GoOptions.NoPoll` leave the polls
   out, for tests and measurements only; the CLI has no flag.

## Consequences
- A GC stop the world waits at most one loop iteration for a hybrid
  loop (0.058 ms measured during an 11.5 s loop, as for Go code);
  sysmon's time slice applies to hybrid loops.
- Cost: 0 to 7 percent on the hybrid benchmarks (median 2): sort 2.0,
  nbody 0, collatz 3.9, sieve_split 5.4, interp_flat 2.0 percent;
  hybrid builds stay 9 to 31 percent faster than the Go backend. Loops of
  4 to 8 instructions pay the most.
- TestHybridPreemption (internal/gen) checks the bounds (stop the world
  under 10 ms, a goroutine's gap under 100 ms at GOMAXPROCS=4 and 1, the
  race detector, and that a build without polls fails them). On a
  loaded host a missed bound is compared with the xo_pure build's figures
  under the same load, capped at a quarter of the loop (2026-10-13,
  gotchas/hybrid-preemption-stw-under-load.md).
- Hybrid builds still exist for arm64 only; amd64 builds use the Go
  versions (`xo_pure.go`), which Go preempts asynchronously.
- The remaining objection of decision 0084 to a default is the selection
  (call heavy functions slower than Go), not preemption.

## Alternatives
- **Exact compare with stackPreempt** (`add x16, x16, #1314; cbz`):
  rejected, 24 percent on a seven instruction loop against 1 percent for
  the bit test.
- **A back edge counter** polling every 1024 iterations: as many hot
  instructions as the load plus a reserved register; no faster
  (sieve 1.062x against 1.071x).
- **An rt owned flag set by a timer** (a sysmon of our own): the same
  load and branch, plus a goroutine or thread to keep it, and it would
  not see a stop the world or a stack scan unless the runtime set it.
- **Chunking from Go** (point 4): rejected as the mechanism; strip mining
  counted loops so only every 4096th iteration polls is the ranked next
  step for tight counted loops.
- **Asynchronous preemption of the assembly**: needs the runtime to treat
  the functions as Go (FuncFlagAsm is set by the assembler for every
  TEXT symbol); not possible without a modified toolchain.
