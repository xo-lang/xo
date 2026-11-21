# clang took minutes on one long function: InstCombine's code sinking moves debug value records along a chain

Status: Observed (2026-11-21, task #586; found through #583)

Symptom: `xo build --backend=llvm` of golden pass_const_edges spent
108 s in clang on the Mac and 235 s on linux/arm64 (clang 23), for a
`main` of 1,521 statements `h = h * 31 % m + (op(k, c) % m).abs()`.
`clang -O2 -ftime-report` on the kept IR (XO_KEEP_ASM=1 with
XO_BUILD_DIR): InstCombine 42 s, EarlyCSE 29 s, AlwaysInliner 14 s of
93 s; code generation under 2 s. Time grew with the square of the
chain: 300 statements 5 s, 600 8 s, 1,200 29 s, 1,300 34 s.

Cause: the debug value records xo emits per assignment
(`llvm.dbg.value`, one per `h = ...`) and InstCombine's code sinking. A
stack sample of the compiler (`clang -fintegrated-cc1`, macOS `sample`)
had its time in InstCombinerImpl::tryToSinkInstructionDbgVariableRecords,
DIExpression::getFragmentInfo and MetadataTracking::track. Sinking an
instruction into its user's block moves the records that name it back
onto its operands; along a chain where each statement uses the last
one's value a record is moved again at every step, and EarlyCSE and the
inliner then walk what was left. Measured on the IR with sed:

| IR | clang |
|---|---|
| as emitted | 93 s |
| no `llvm.dbg.value` at all | 11 s |
| no records of `h` only (1,521 of 17,656) | 14 s |
| no records with a constant value (6,058) | 95 s |
| a distinct variable per record of `h` | 147 s |
| every 4th record of `h` | 24 s |
| `-mllvm -instcombine-code-sinking=false`, every record kept | 8 s |

The helpers kept as calls (no call site alwaysinline) left InstCombine
at 64 s alone: the chain, not the inlining.

Not xo's IR alone: a plain C main with the same overflow checked chain
(`__builtin_mul_overflow`, `clang -O2 -g`, Apple clang, one run each)
took 1.8, 3.1 and 22.0 s at 400, 800 and 1,600 statements, 0.6, 2.3
and 5.8 s with sinking off, 0.7, 2.2 and 9.0 s without `-g`. xo's IR
is about 5 times slower than C's at the same length (108 s at 1,521
against 22 s at 1,600), not yet explained (task #593).

Fix: every optimized LLVM module is compiled, and with LTO linked, with
`-instcombine-code-sinking=false` (compiler/xo split.xo giant_fn_flags,
ir_fn_size; over GIANT_FN_BLOCKS, 8,000, the AArch64 machine combiner
is off too, as before; an -O0 build gets neither). Every debug record
stays. After: the golden 4.2 s of clang on the Mac; chains of 150, 300,
600, 1,200 and 4,000 statements 4.0, 4.8, 4.1, 4.5 and 6.1 s. Guard:
internal/llvm TestGiantFnCompileTime (2,000 statements: 102.5 s of user
CPU time before, 4.9 s after, budget 75 s).

No threshold (owner, 2026-11-21): the first version turned sinking off
only past 2,000 blocks in one function, a line drawn from one code
shape, with a switch per module (clang's options are per module, and a
whole program build is one module). Task #590 measured what sinking
buys: off against on, linux/arm64, whole program LLVM builds, wall time
1.002 as the geometric mean of 71 workloads (worst portable arith8
1.059), xo's own build 1.009 within noise, `.text` 0 to 288 bytes
smaller per bench (bench/results/llvm-code-sinking-2026-11-21.md). So
it is off for every size.

Avoid: for a slow clang step, keep the IR and read `-ftime-report`
before any guess, then sample the compiler in process
(`-fintegrated-cc1`; the driver alone only waits); try the candidate
IR feature by sed on the kept IR before changing the emitter.
