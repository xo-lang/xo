# V8's optimizing tier, not the module's compile, costs the time and memory of a large wasm module

Status: Observed (2026-10-12), selfhost/wasm stage 3 (decision 0179).

**Symptom.** The wasm build of the self hosted checker (31.6 MB then)
took 7.0 s and 4.8 GB under Node for seven small goldens, against
0.06 s natively. Compiling the module alone (`new WebAssembly.Module`,
eager) took 0.84 s and 1.0 GB, so the run looked slow, and the earlier
notes blamed "compiling the module at instantiation".

**Cause.** V8 compiles every function with Liftoff first and recompiles
the hot ones with TurboFan in the background. TurboFan on functions of
hundreds of KB and thousands of locals (the dispatch loop of a long
`Checker` method) is what takes the seconds and the gigabytes:

| node flag (same module, same input) | time | max RSS |
|---|---|---|
| default | 7.03 s | 4.83 GB |
| `--no-wasm-tier-up` | 7.28 s | 5.41 GB |
| `--liftoff-only` | 1.80 s | 1.12 GB |
| `--no-liftoff` | 8.83 s | 7.69 GB |

(bench/results/wasm-2026-10-12.md, "Stage 3".)

**Fix.** Make functions smaller (stage 3 of decision 0179: large `var`
parameters by reference, shared scratch locals); with the module at
9 MB the same run took 1.9 s and 3.5 GB. When measuring a module's cost
in V8, separate the compile (`new WebAssembly.Module`) from the run,
and try `--liftoff-only` before concluding the code is slow. A page
cannot set V8 flags, so in a browser the fix has to be in the module.
