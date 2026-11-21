# 0731: LLVM builds compile with InstCombine's code sinking off

Status: Accepted (owner, 2026-11-21, xw decision #241; tasks #586, #590)

## Context

Golden pass_const_edges took clang 108 s on the Mac and 235 s on
linux/arm64 for one `main` of 1,521 chained statements (task #586,
found through #583). The time was in InstCombine's code sinking: when
it sinks an instruction into its user's block it moves the debug value
records that name it, and along a chain of statements that each use the
last one's value a record is moved again at every step, so the cost
grows with the square of the chain
(gotchas/llvm-instcombine-sinking-debug-records-quadratic.md). A plain
C function of the same shape grows the same way under `clang -O2 -g`
(1.8, 3.1 and 22.0 s at 400, 800 and 1,600 statements), so it is how
the pass works on the clang users have, not only xo's IR.

The first fix turned sinking off only for a module with a function of
more than 2,000 basic blocks. That line came from one code shape, and
clang's options are per module, so one long function switched the pass
off for every function beside it (a whole program build is one module).

Task #590 measured what sinking buys at run time, off against on, on
linux/arm64 host 1 with whole program LLVM builds
(bench/results/llvm-code-sinking-2026-11-21.md): wall time 1.002 as
the geometric mean of 71 workloads (worst portable arith8 1.059, best
ctx await 0.972), the LLVM-built compiler building compiler/xo 1.009
within noise, `.text` 0 to 288 bytes smaller per bench and 864 bytes
smaller on xo.

## Decision

Every optimized LLVM build (`-O1` and above, each module's compile and
the LTO link) passes `-instcombine-code-sinking=false` to clang, on
every architecture and at every function size. An `-O0` build is
unchanged. Debug information is unchanged: every debug value record is
still emitted.

## Consequences

- No function size makes clang's time grow with the square of a chain
  of statements through this pass; TestGiantFnCompileTime (internal/llvm)
  budgets a 2,000 statement `main` at 75 s of CPU time (4.9 s with the
  option, 102.5 s without).
- One measured cost: bench/perf/portable arith8 runs about 5% slower
  (1.046 to 1.059). 70 of the 71 workloads run under 100 ms, where
  `xo bench --ab` gives no verdict; the figures are 30 round medians.
- The option is an internal LLVM flag (`-mllvm`), not a stable clang
  interface: a clang that drops or renames it fails the build, which
  the LLVM goldens show on every host's clang.
- xo's IR is still about 5 times slower in clang than the C twin at the
  same chain length; task #593 looks for the reason.

## Alternatives

- A block threshold (2,000 blocks, the first version of #586): keeps
  sinking for small functions, which measured as nothing, at the price
  of a cliff and a per module switch.
- Fewer debug value records from xo: every 4th record of the chained
  variable still took 24 s against 93 s, and a debugger then shows
  stale values for a reassigned variable.
- A fix in LLVM: the right place, but it reaches users only with a
  newer clang, and a contribution to an outside project is the owner's
  decision.
