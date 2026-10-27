# nbody against Go (milestone 0.34.0 criterion 4, 2026-10-27)

release-vs-go-v0.33.0.md lists nbody (bench/perf/llvm/xo/nbody, 5M
`advance` calls over 5 bodies) at 1.05x Go. This records what was measured
and ruled out, and why it is parked: most of the gap is Go's free fused
multiply-add, which Xo does not do (decision 0287, the owner again on
2026-10-27: floats stay strict, no automatic fusion, no fast math; only an
explicit `x.fma(y, z)` fuses).

Machine: the arm64 Mac (Apple M-series). xo at 3b0f706ac's parent (main of
2026-10-27). `xo bench --ab`, 10 interleaved rounds, quiet measure window.

## Where the cycles are

| | instructions | cycles | IPC |
|---|---|---|---|
| Go 1.26.1 | 3.015 G | 648 M | 4.65 |
| xo (native arm64) | 2.616 G | 685 M | 3.82 |

xo runs 13% fewer instructions and 5.6 to 6.9% more cycles (from 1.056 to
1.069 across the runs below): about 7 cycles more per `advance` call (137
against 130). The loop is latency bound, not instruction bound.

Go's inner loop (`go tool objdump -s main.advance`) has 8 `fmadd`/`fmsub`
per pair and 3 in the position loop: `dx*dx + dy*dy + dz*dz` and every
`v -= d * m` is one fused instruction. Xo keeps each multiply and add
apart (each rounds, as the language requires), packs pairs into NEON lanes
instead (decision 0323's slp_f64; with it off nbody is 1.047x slower), and
so has fewer instructions on a longer dependence chain.

## Ruled out (ceilings, each an `--ab`)

- **SLP re-pack on the loop-carried chain.** The inner loop rebuilds
  `bs[i].vx, vy` as a vector each pair (`zip1`, `fsub.2d`, two `dup`).
  Carrying them in one vector register across the loop (asm edited
  through a cc wrapper) cut instructions 4.8% and made cycles 1.2% worse:
  the shuffles are not on the critical path.
- **Store-to-load forwarding.** The i row's `vx, vy` are stored with
  `stp` (two 8 byte stores) and read back by the position loop with one
  16 byte `ldr q`, which cannot forward from two stores. Storing them as
  one `str q` instead: cycles 1.007 (worse).
- **The loop body itself.** llvm-mca 23.1.3 (-mcpu=apple-m4) puts xo's
  inner pair loop at 19.3 cycles an iteration against Go's 23.1 in steady
  state, and lower at nbody's real trip counts (1 iteration: 86 against
  103 cycles; 4: 139 against 154). A whole `advance(n = 5)` call laid out
  straight (prologues, 4+3+2+1 inner bodies, epilogues, the position loop
  five times): 229 cycles for xo against 283 for Go. The model, which has
  no memory dependences and a fixed fused-op latency, ranks xo ahead; the
  hardware ranks Go 5% ahead.

## Parked

No non-FMA cause was found that a change could remove. The remaining gap is
consistent with fusion: Go's `fmadd` chains are one rounding and one
latency each where Xo's are two, on a chain that the M-series' out-of-order
window cannot hide with 1 to 4 iterations per inner loop. Reopening it
would mean a reason to revisit decision 0287, which the owner declined on
2026-10-27.

Files: the ceiling scripts and edits were scratch (cc wrappers editing
prog-darwin.0.s); the method is in agent-tools.md ("Measuring").
