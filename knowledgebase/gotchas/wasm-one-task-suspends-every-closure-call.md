# One task makes every closure call a wasm suspension point

Status: Observed (2026-10-29)

**Symptom.** The playground compiler (compiler/wasm built for wasm) grew from 19.7 MB (2026-10-17) to 87.9 MB at 0.34.0, 10.2 MB brotli against 1.6. Its function count grew 1.75x, its code 4.6x: ssa.Builder.pattern alone was 842 KB, 15,544 locals saved and restored at each of 36 suspension points.

**Cause.** compiler/wasm gen.xo find_susp marked every function with a call through a function value (CallValue) as suspending once any function of the program made a blocking runtime call (rt3_blocking: task_await, scope_close, atomics, channels, mutexes, sleeps), and every caller of one transitively. When ssa's fault_scans and cleanup_funcs started running as tasks, 707 functions (55.4 of the 87 MB) gained frame saves. The wasm module's name section names functions $fN; the .wat (XO_KEEP_WAT=1) has `;; Name` above each function.

**Fix.** A CallValue suspends only when a function a function value can name (Func.indirect, a Rebind target) with the same call_indirect parameter types (the leaves of every argument) can suspend, iterated with the callers to a fixed point. Homes never apply to an indirect function, so its wasm parameters are exactly its parameters' leaves.

**Avoid.** A size jump of a wasm build: count functions with `global.get $sstate` in the .wat and their bytes before looking at codegen.
