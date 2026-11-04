# rt.fn_addr on wasm built native-only exports into every module

Status: Observed (2026-11-03, a draft that lowered rt.fn_addr to a table index)

Symptom: with rt.fn_addr lowered in a wasm build to the named C ABI
export's table index (as rt.wasi_hook does), about 30 goldens stopped
building for wasip2/wasm: `the wasm backend does not support the call
rt.stack_switch, in sched.fiber_direct` (or sched.fiber_park).

Cause: std code that a wasm build reaches names native-only exports
through rt.fn_addr in arms the passes keep (ctx.xo stores fiber_main,
worker_main and stdin_pump_main; cabi.xo's heap and fault hooks).
As a fault, rt.fn_addr built nothing; as a FuncAddr it builds each named
export, and those reach the native fiber switch, which has no wasm form.
compiler/ssa call.xo's comment on rt.wasi_hook said so before the draft.

Fix: reverted; rt.fn_addr stays a call that faults if it runs in a wasm
module. rt.wasi_hook names the exports a wasm arm needs one by one.

How to avoid: before lowering an intrinsic that names a function, count
the call sites a wasm build keeps (all goldens as wasip2 commands, not a
subset); a per-export fallback (build the export, drop it and keep the
fault when its body cannot lower) is the way to lower rt.fn_addr itself.
