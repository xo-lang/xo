# 0084: Hybrid builds: hot leaf functions as Go assembler, an option

Status: Accepted (2026-10-09). Extends decisions 0040 (performance path),
0049 (arm64 backend), 0054, 0060, 0075. Implemented in
`internal/arm64/goasm.go` (Go mode), `internal/arm64/gosyntax.go`,
`internal/gen/hybrid.go`, `internal/build/hybrid.go`, `std/rt/hybrid.go`,
`cmd/xo` (`--hybrid`). Evidence: `bench/results/go-asm-hybrid-2026-10-09.md`.

## Context
The native arm64 backend runs compute code at LLVM speed but has no
Go runtime: its HTTP, TLS, scheduler, and memory management are its own
(decisions 0068, 0074, 0083). The Go backend has all of Go's runtime but
loses on tight loops (nbody 1.5x, collatz 1.4x of native). The question
was whether the hot leaf functions of a Go backend build can be compiled
by the arm64 backend into Go assembler and linked in, getting native
speed where it matters and Go everywhere else.

## Decision
1. **`xo build --hybrid`** (Go backend, programs): free functions of the
   main module that are eligible and have a loop are compiled to Go
   assembler (ABI0) by the arm64 backend's Go mode and called from a Go
   wrapper. Eligible: no allocation, no reference count, no Str;
   parameters and results Int, Float, Bool, structs of Int and Float
   fields, Lists of those (`var` only on Lists); calls only to eligible
   functions and the inline List and scalar methods; no `ensures`
   clause, no struct with an invariant (the SSA builder checks neither;
   amended 2026-10-09 by decision 0085: it checks both, and the
   exclusion is gone).
   Anything else, or a program outside the SSA subset, stays Go; the
   build never fails because of `--hybrid`.
2. **An option, not the default.** Eligible loops run at native speed
   (programs 12 to 31 percent faster than the Go backend, within 3 to 8
   percent of `--backend=arm64`), but few functions qualify (none in the
   HTTP benchmark, 4 of 77 golden programs), call heavy recursion is
   slower (fib 1.49x), and the runtime cannot preempt assembly, so a long
   loop delays GC stop the world.
3. **The Go versions stay** in `xo_pure.go` under tag `xo_pure` (and on
   targets other than arm64): the fallback and the reference of the
   differential tests, which run on every eligible golden and on random
   kernel programs, with `go vet -asmdecl`.
4. Faults raise exactly the Go backend's fault (`rt.HybridFault`);
   fault stacks lose the assembly frames. Float results may differ from
   the Go backend in the last bit where Go fuses a multiply and an add.

## Consequences
- The arm64 backend has a second output convention; changes to
  instruction selection reach hybrid builds too, and
  TestGoSyntaxEncoding fails on any new instruction form the translator
  does not handle (the function then stays Go, with the reason).
- `xo build --hybrid` runs the native check and SSA in addition to the
  Go backend (tens of milliseconds).
- Agents that want native speed for a numeric kernel in a server can
  get it without leaving Go's runtime, by keeping allocation out of the
  hot function (`bench/perf/hybrid/xo/sieve_split`, `interp_flat`).
- The native backends' missing `ensures` check surfaced
  (gotchas/native-ensures-unchecked.md).

## Alternatives
- **Make it the default for eligible functions**: rejected until loops
  in assembly poll for preemption (gotchas/go-asm-not-preemptible.md)
  and the selection is proven never slower on more programs.
- **Drop it**: rejected; the speedups on numeric kernels are real and the
  cost is an opt-in flag.
- **ABIInternal assembly** (arguments in registers): only the Go runtime
  may write it; a private register convention between assembly
  functions would need its own stack growth path, since morestack
  clobbers every register (gotchas/go-asm-values-across-calls.md).
- **Raw machine code (`WORD`) assembled by cc**: needs a C toolchain in Go
  builds and hides the code from `go vet`; the translator is checked
  against cc's encodings instead.
- **cgo calls into native code**: tens of nanoseconds per call, and Go
  backend builds are `CGO_ENABLED=0` (decision 0002).
