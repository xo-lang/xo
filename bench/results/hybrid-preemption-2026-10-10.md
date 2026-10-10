# Preemption of hybrid assembly loops (2026-10-10)

Question: a loop in Go assembler (`xo build --hybrid`, decision 0084)
cannot be preempted by the Go runtime. How long does it stall the GC and
other goroutines, and what is the cheapest way to bound that?

Answer: before this change a hybrid loop held its P for as long as it
ran: a GC stop the world waited 11.6 s behind an 11.5 s loop. With a poll
of `g.stackguard0` on every loop back edge (two instructions, decision
0131) the longest stop the world is 0.058 ms and a goroutine sharing the
only P gets it back every 30 ms, as with plain Go. The polls cost 0 to 7
percent on the hybrid benchmarks (median 2 percent). Chunking the loop
from Go costs less where it applies (0.6 percent on sieve at 4096 values
per call) but bounds nothing in general.

Apple M4 Pro, darwin/arm64, Go 1.26.1; host load average 2 to 5 (other
processes). Before: the commit before the polls (`xo-before`, with only
the RC verifier fix below); after: branch head.

## Latency

`internal/gen/testdata/hybrid_preempt`: `spin(rounds)` (Collatz step
counts over `rounds` starts, in assembly) runs in a task while `main`
pushes 2000 Ints into a fresh List and sleeps 100 us in a loop until spin
is done, recording its longest gap; `GODEBUG=gctrace=1` gives each GC's
stop the world phases (the first and third clock numbers). 66M rounds
(about 11.5 s of loop):

| Build | GOMAXPROCS | spin + main ms | main's worst gap ms | main iterations | GCs | longest stop the world ms |
|---|---|---|---|---|---|---|
| hybrid, no polls | 4 | 11629 | 11560 | 458 | 1 | 11559 |
| hybrid, polls | 4 | 12794 | 0 | 94944 | 201 | 0.058 |
| Go backend | 4 | 15100 | 0 | 112051 | 237 | 0.070 |
| hybrid, no polls | 1 | 11009 | 11009 | 2 | 0 | |
| hybrid, polls | 1 | 11060 | 30 | 388 | 0 | |
| Go backend | 1 | 12916 | 60 | 441 | 0 | |

Without polls the first GC's sweep termination stop the world waits for
the whole loop, and every goroutine that allocates waits with it
(458 iterations of main in 11.6 s). With GOMAXPROCS=1 main runs twice.
With polls the loop yields within one iteration of a stop request
(stop the world 0.058 ms, as Go's own 0.070 ms) and sysmon's 10 ms time
slice applies (main's 30 ms gap at GOMAXPROCS=1 is its share of the P
against the loop; the Go build shows 60 ms). The hybrid run at
GOMAXPROCS=4 is 10 percent slower with polls than without because the
loop now stops for 201 GCs (each a stop the world and a stack scan)
instead of 1, and their mark work shares the CPU; spin alone is 1
percent slower (below).

TestHybridPreemption (internal/gen) runs this with 2M rounds (-short)
or 12M rounds and checks: stop the world under 10 ms at GOMAXPROCS=4,
main's gap under 100 ms at GOMAXPROCS=4 and 1, the same under the race
detector (full suite), and that the build without polls
(`Options.HybridNoPoll`) misses both bounds (full suite; its stop the
world was 1904 ms).

## The poll

At each back edge (the end of a block with an edge to a block on the
depth first search stack):

```
ldr  x16, [x28, #16]      // g.stackguard0
tbnz x16, #63, poll       // stackPreempt (-1314) and stackFork have bit 63 set
```

`poll`, after the function body, stores the registers live across it,
calls `xoHybridYield` (`runtime.Gosched`), reloads them, and jumps back.

Variants on `spin` (6M rounds, median of 2 to 3 runs, ms):

| Variant | ms | vs no poll |
|---|---|---|
| no poll | 899 | 1.00x |
| `ldr; add #1314; cbz` (exact compare, flag free) | 1115 to 1145 | 1.24x |
| `ldr; tbnz #63` | 905 to 933 | 1.01x |
| poll in the outer loop only (unbounded) | 896 to 924 | 1.00x |

The exact compare adds a dependent ALU operation to every iteration of a
seven instruction loop; the bit test does not. Variants on sieve_split's
`mark` (hand edited assembly, 11 interleaved runs, whole program):

| Variant | ms | vs no poll |
|---|---|---|
| no poll | 182.5 | 1.000x |
| `ldr; tbnz` on every back edge (the design) | 195.5 | 1.071x |
| outer loop only (inner loop unbounded) | 189.4 | 1.038x |
| counter: `add #1, x25; tbnz #10, x25` (load every 1024) | 193.8 | 1.062x |
| chunking from Go, 4096 values of i per call, no polls | 182.3 | 0.999x |

A counter is as many instructions as the load and needs a register: no
gain. The cost is instruction count in loops of 4 to 8 instructions.

## Chunking from Go

`bench/perf/hybrid/xo/sieve_chunk`: the same sieve with `mark_range(var
flags, n, lo, hi)` (assembly, no polls) called from a Go loop over
chunks of k values of i (11 interleaved runs):

| Build | ms | vs sieve_split without polls |
|---|---|---|
| sieve_split, no polls | 180.8 | 1.000x |
| sieve_split, polls | 193.6 | 1.070x |
| chunks of 1 | 354.7 | 1.961x |
| chunks of 64 | 189.9 | 1.050x |
| chunks of 4096 | 182.0 | 1.006x |
| one chunk (k = 1e9) | 181.8 | 1.005x |
| chunks of 4096, polls | 195.3 | 1.080x |

Chunking is cheap when the work per chunk is large, but it bounds
latency only if every chunk is short: `mark_range`'s inner `while` marks
n / i elements per i (5M for i = 2), and loops like spin's `while n != 1`
have no bound at all. Doing it in the compiler means making every loop
resumable (its live values out to the wrapper and back), with a budget
counter on every back edge: the counter row above, plus the exits. So
the general mechanism is the poll; chunking is the model for the next
step (strip mining of counted loops, below).

## Overhead on the hybrid benchmarks

`XO_BASE=xo-before REPS=9 bench/perf/hybrid/run.sh` (the base column is
the hybrid build without polls, interleaved with the others):

| Workload | in assembly | Go ms | hybrid ms | arm64 ms | hybrid/Go | hybrid/arm64 | Go Mcyc | hybrid Mcyc | arm64 Mcyc | base ms | hybrid/base |
|---|---|---|---|---|---|---|---|---|---|---|---|
| sort | quicksort | 270.8 | 234.9 | 215.9 | 0.87x | 1.09x | 1103 | 957 | 880 | 230.3 | 1.020x |
| nbody | advance, energy, offset | 255.8 | 177.5 | 173.5 | 0.69x | 1.02x | 1022 | 707 | 691 | 177.5 | 1.000x |
| sieve | no hot function is eligible | 149.3 | 150.6 | 108.1 | 1.01x | 1.39x | 606 | 603 | 426 | 150.6 | 1.000x |
| interp | no hot function is eligible | 174.8 | 174.8 | 46.5 | 1.00x | 3.76x | 695 | 693 | 177 | 174.4 | 1.003x |
| fib | no hot function is eligible | 232.9 | 231.9 | 220.9 | 1.00x | 1.05x | 930 | 925 | 886 | 231.7 | 1.001x |
| collatz | steps | 552.2 | 462.9 | 430.6 | 0.84x | 1.08x | 2410 | 1968 | 1905 | 445.7 | 1.039x |
| sieve_split | mark | 218.6 | 198.4 | 101.7 | 0.91x | 1.95x | 878 | 803 | 414 | 188.2 | 1.054x |
| interp_flat | run | 102.8 | 86.4 | 67.6 | 0.84x | 1.28x | 407 | 343 | 258 | 84.7 | 1.020x |

With polls the hybrid builds stay 9 to 31 percent faster than the Go
backend (sort 0.87x, nbody 0.69x, collatz 0.84x, sieve_split 0.91x,
interp_flat 0.84x). Workloads without assembly are unchanged (noise
within 0.3 percent).

## Correctness

- TestGoldenHybrid (4 golden programs, every eligible function in
  assembly) and TestHybridRandom (random kernels against their `xo_pure`
  builds) pass with the polls; `go vet -asmdecl` passes.
- TestGoSyntaxEncoding has the two new forms (`ldr x16, [x28, #16]` as
  `MOVD 16(g), R16`, `tbnz x16, #63`).
- A function that polls is no longer `//go:noescape` (its slices' arrays
  stay in the heap, which does not move, while the yield may move the
  goroutine's stack); a function with a poll and no other call stays
  NOSPLIT when its frame is at most 256 bytes.

## Found on the way

The hybrid build failed for every program that starts a task: the RC
verifier, at the `rt.unwinding` check of a task runner built without
fault unwinding (hybrid's SSA), dropped the entry block's parameters as
if they were an inlined call's results, and reported `v1 is not alive`;
the build fell back to plain Go with "no assembly". Fixed in
`internal/ssa/verify_rc.go` (gotchas/ssa-rc-verifier-entry-params.md).

## Next steps (ranked by likelihood of paying off)

1. **Strip mine counted loops** (`for i in a..b` and `while` loops with
   a loop invariant bound and step): run the loop in strips of 4096
   iterations with the poll on the strip's back edge only, so the inner
   back edge carries nothing (the chunks of 4096 row: 1.006x against
   1.070x). In `insertPolls`, use lsr.go's induction variables
   (`s.lsr`) to find the bound; target sieve_split's 7 percent.
2. **Skip the poll in loops that already call a function with Go's stack
   check** (the callee's prologue polls): sort's quicksort recursion.
   Small, measure sort.
3. **One poll per loop nest when the inner loop's trip count is
   bounded** by a constant (ranges.go intervals), e.g. nbody's loops over
   5 bodies.
4. **Make hybrid the default for eligible loops** (the condition 0084
   set): measure more programs with the polls first, and the selection
   must never be slower than Go (fib).
