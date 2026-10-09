# Hybrid builds: hot leaf functions as Go assembler (2026-10-09)

Question: can Xo get native code speed for hot leaf functions while
keeping the Go runtime (net/http, TLS, the scheduler, the GC), by
emitting those functions with the arm64 backend as Go assembler linked
into Go backend builds?

Answer: yes, for loops over numbers and Lists of flat structs. Those
functions run at the native backend's speed (the same instruction
selection and allocation; the whole programs land within 3 to 8 percent
of `--backend=arm64`), and the programs got 12 to 31 percent faster than
the Go backend. But few functions qualify: none in the HTTP benchmark,
4 of the 77 golden programs, and call heavy recursion is slower than Go.
Decision 0084 keeps it as an option (`xo build --hybrid`), not the
default.

## What was built

- **Go mode of internal/arm64** (`goasm.go`): eligible functions (no
  allocation, no reference count, no Str; parameters and results Int,
  Float, Bool, structs of Int and Float fields, and Lists of those, a
  `var` only on a List; calls only to other eligible functions and the
  list and scalar methods lowered inline) are emitted by the existing
  pipeline with these changes: a List is its Go slice's base and its
  length a parameter register, arguments and results in memory (ABI0),
  Go's reserved registers (x18, x26 to x30) never allocated, no callee
  saved registers (values live across a call are stored and reloaded
  around it), NOSPLIT unless the function calls an Xo function (then Go's
  assembler inserts the stack check, so deep recursion grows the
  goroutine stack), `NO_LOCAL_POINTERS`, faults through a Go function
  that raises the Go backend's fault. The GNU text is translated to Plan 9
  syntax line by line (`gosyntax.go`); TestGoSyntaxEncoding assembles all
  108 forms both ways and compares the machine code.
- **Selection**: eligible functions with a loop. Recursive fib (no loop)
  in assembly ran 352.9 ms against the Go backend's 236.7 ms (1.49x
  slower): ABI0 passes arguments in memory and every call runs the stack
  check prologue, while Go's register ABI does neither.
- **internal/gen**: the wrapped function calls the assembly with its
  Lists' slices after making each written `var` List unique (as the Go
  backend does before an element write); its Go version moves to
  `xo_pure.go` (tag `xo_pure`, or a non arm64 target).
- **Not eligible**, with the reason in `GoProgram.Rejected`: methods,
  generic instances, other modules, `ensures` clauses (the native
  backends skip them: gotchas/native-ensures-unchecked.md), structs with
  invariants, anything holding Str, Map, enums with heap payload, or
  allocating (`push`, list literals).

## Method

`bench/perf/hybrid/run.sh`, REPS=7: per workload the Go backend (`xo
build`), the hybrid build (`xo build --hybrid`), and the native backend
(`--backend=arm64`) run interleaved under `/usr/bin/time -l`; the table
shows medians of wall time (process start included) and of "cycles
elapsed". Outputs agree in every run. Apple M4 Pro, darwin/arm64, Go
1.26.1. No other agent ran; the host's load average fell from 4.97 to
2.20 during the run (other processes). Commit of the run: the branch's
final commit.

## Results

| Workload | in assembly | Go ms | hybrid ms | arm64 ms | hybrid/Go | hybrid/arm64 | Go Mcyc | hybrid Mcyc | arm64 Mcyc |
|---|---|---|---|---|---|---|---|---|---|
| sort | quicksort | 269.8 | 229.6 | 213.3 | 0.85x | 1.08x | 1107 | 946 | 868 |
| nbody | advance, energy, offset | 258.5 | 177.7 | 171.6 | 0.69x | 1.03x | 1031 | 708 | 684 |
| sieve | none eligible | 148.6 | 147.0 | 99.2 | 0.99x | 1.48x | 609 | 600 | 402 |
| interp | none eligible | 174.8 | 175.1 | 47.4 | 1.00x | 3.69x | 696 | 696 | 179 |
| fib | none hot (no loop) | 233.0 | 233.8 | 216.7 | 1.00x | 1.08x | 925 | 933 | 874 |
| collatz | steps | 606.4 | 447.9 | 430.4 | 0.74x | 1.04x | 2687 | 1977 | 1907 |
| sieve_split | mark | 213.6 | 187.8 | 107.3 | 0.88x | 1.75x | 860 | 777 | 435 |
| interp_flat | run | 108.8 | 85.0 | 72.6 | 0.78x | 1.17x | 435 | 334 | 268 |

- **sort** (the comparison loop: `less` is inlined into `quicksort`, which
  recurses): 15 percent faster than Go, 8 percent behind native: each of
  the about 2M recursive calls pays ABI0 and the Go prologue.
- **nbody** (`advance`, Float math over a List of structs): 31 percent
  faster, within 3 percent of native.
- **sieve** and **interp** as written are not eligible: `sieve` pushes
  into the List it marks, `run` holds a `List[Op]` of an enum and pushes
  and pops a stack. The variants in `bench/perf/hybrid/xo` do the same
  work with the allocation moved out of the hot function:
  `sieve_split` (the marking loop as `mark(var flags, n)`) and
  `interp_flat` (the program as Int pairs, the stack preallocated).
- **sieve_split**: the hot function itself is as fast as native (the same
  instructions); the program is 12 percent faster than Go but 1.75x of
  native because 10M pushes per round stay Go code. A CPU profile of 3
  runs each (`XO_CPUPROFILE`): `F_mark` 280 ms against `xoasm_mark` 230
  ms; the rest is the Go push loop.
- **interp_flat**: 22 percent faster than Go, 1.17x of native: Go mode
  has no jump tables (a table is data in the text section), so the
  10 way `match` is a compare chain (decision 0051 measured the table's
  worth on interp).
- **HTTP** (`bench/perf/http/xo`): no function qualifies. Every hot path
  takes or returns Str, JSON, a Map, or the `Mutex[State]`, and the store
  operations are methods. The JSON encode path allocates; nothing to
  measure.

## Correctness

- TestGoldenHybrid: the 4 golden programs with an eligible function
  (`arith_overflow`, `closures`, `div_zero`, `loops`), every eligible
  function in assembly, print exactly want.txt, overflow and division
  faults included. Of the 77 golden programs (Build mode), 34 are
  outside the SSA subset and 39 have no eligible function. Writing it
  found that the native backends skip `ensures`
  (gotchas/native-ensures-unchecked.md).
- TestHybridRandom: 12 random programs of 15 kernels (Int, Float, Bool,
  and struct Lists, `var` writes, recursion, nested loops) with random
  data, each built with the kernels in assembly and with tag `xo_pure`,
  print the same; the seeds cover index, overflow, division by zero, and
  Float to Int faults. It found the Go backend's multiply add fusion
  (gotchas/go-backend-fma-contraction.md).
- `go vet -asmdecl` passes on every build directory (frame sizes,
  argument names and offsets, access widths).

## What it costs

- **No preemption inside assembly**: the Go runtime never preempts
  assembly asynchronously, so a long loop delays GC stop the world
  (gotchas/go-asm-not-preemptible.md). Fine for kernels that return
  within a millisecond or so; not for a server's long loops.
- Fault stacks lose the frames of assembly functions (the wrapper shows
  instead); messages and positions are the same.
- Float results can differ from the Go backend in the last bit where Go
  fuses `a * b + c`.
- A build runs the native check and SSA too (tens of milliseconds).

## Conclusion

Worth it as an opt-in for numeric kernels: loops over Int, Float, Bool,
and Lists of flat structs that do not allocate, the kind nbody, sort,
collatz, and a byte code dispatch loop are. Those reach native speed
with the Go runtime kept. Not worth it for web service code (nothing
qualifies), for loop free or call heavy functions (Go's register ABI
and inliner win), or as a default while assembly cannot be preempted.

## Next steps (ranked by likelihood of paying off)

1. **Preemption poll at back edges**, so a hybrid loop is a good
   citizen: in `goEmit`, at each loop latch, `MOVD 16(R28), R16; CMP
   $-1314, R16; BEQ` to a cold block that saves live registers and calls
   a Go `xoHybridYield` (runtime.Gosched). Measure its cost on
   sieve_split and nbody and the GC pause with a concurrent allocating
   task (`GODEBUG=gctrace=1`). This is the condition for a default.
2. **Jump tables in Go mode**: a `DATA`/`GLOBL` table of branch offsets
   in RODATA and `ADR` to a base label (or a table of `JMP` instructions
   indexed with `ADD R9<<2`); target interp_flat's 1.17x of native.
3. **Wider eligibility**: read only Str parameters as (base, len) for
   parsers and scanners; Lists of enums without heap payload (interp's
   `List[Op]`); methods of the main module.
4. **Lower `ensures` in the SSA builder** (fixes the native backends
   too, and lifts the exclusion).
5. **Cheaper calls between assembly functions**: a register convention
   for calls from one assembly function to another would need its own
   stack growth path (morestack clobbers registers), so measure first
   how much of sort's 8 percent is the prologue (`XO_CPUPROFILE`).
