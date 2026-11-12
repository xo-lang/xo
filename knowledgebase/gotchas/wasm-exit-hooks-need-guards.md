# An exit hook registered through rt.fn_addr does not build on wasm

Status: Observed (2026-11-12, log.configure_handler's release at exit, decision 0617 C)

Symptom: after adding an atexit hook (atexit.set_log(rt.fn_addr("log_free")),
atexit.run_log()), the wasm golden became unsupported: `the wasm backend
does not support the call rt.fn_addr`, then, with that guarded, `rt.call_c2`.

Cause: a WASI module has no function addresses (wasm-fn-addr-builds-native-exports.md)
and atexit.xo's run calls through rt.call_c2. Registering and running a
hook are both native-only.

Fix: guard both with `if target.arch != TargetArch.Wasm32` (as
twrun.xo's start does for tw_end), and release the state on wasm where
it is replaced or reset instead (std/native/log.xo configure calls the
release directly). Then check the wasm leak check (TestSelfhostWasm):
a value kept in a static word is a leak at exit unless released.
