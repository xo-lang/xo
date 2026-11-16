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
