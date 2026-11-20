# A runtime guard that is always false on wasm still makes its callee reachable

Status: Observed (2026-11-16, 2358ad34f)

Symptom: after 2358ad34f (task #120, raw FakeNet connections served
natively), `xo build --targets=wasip2/wasm` of every program serving HTTP
(goldens http_server, http_server_request) failed: "the wasm backend does
not support the call `rt.stack_switch` (stage 2), in sched.fiber_direct".
TestWasmCommandGoldens failed on main.

Cause: hconn's in memory paths are behind `let me = conn_me(hc); if me
!= 0 { me_io(...) }`. conn_me returns 0 on wasm, but the wasm backend's
reachability walk does not fold a call's result, so me_io stayed
reachable, and twrun's waits it calls park through the native scheduler's
fiber switch, which WASI 0.2 has not.

Fix: put the `target.arch == TargetArch.Wasm32` test in the callee that
reaches the unsupported code (me_io returns at once there): a constant
target test in the function body prunes, a value computed from one in
another function does not. Check with `xo build --targets=wasip2/wasm`
of an HTTP server golden after a change under std/native/llrt/hconn.xo or
hserve.xo.

## A target test inside an && chain does not prune either (Observed, 2026-11-20, 745cbde2b)

Symptom: after 745cbde2b (task #286, h2_pump yields before its read
tick), 58 wasm goldens failed on main, every wasip2 HTTP server among
them: "the wasm backend does not support the call `rt.stack_switch`
(stage 2), in sched.fiber_park". No gate saw it: the gate's wasm step
runs only the goldens a change touches, and the change touched none.

Cause: the guard was `if a && b && c && target.arch !=
TargetArch.Wasm32 { pool.yield_now() }`. Only an `if` whose whole
condition is the target test is folded; inside a chain the test is a
plain value, so yield_now (a fiber switch) stayed reachable on wasm.

Fix: the target test on its own `if`, the rest nested inside it
(task #492). scripts/gate.sh's wasm step now adds http_h2c,
http_duplex and http_pipe when std/native/http*.xo or
std/native/llrt/http*.xo changes; `OFFGATE_GATE_ENV=XO_GATE_WASM_ALL=1`
runs both wasm runners unfiltered.
