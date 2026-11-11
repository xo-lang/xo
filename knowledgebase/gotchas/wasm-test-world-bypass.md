# Capabilities the wasm backend serves itself bypass the test world

Status: Observed (2026-11-11, Xo Lang 2, decision 0610)

Symptom: std's tests under `xo test --backend=wasm` saw the runner's real
environment (`os.env.get("PATH")` returned the Mac shell's PATH, and
`os.env.set` did not stick), `os.clock.now()` did not move after
`os.clock.advance`, and a ticker's `wait` returned at once. The same
tests pass natively. Before that, every test failed with "no test with
this index" and its failure text was cut by 8 bytes at each end.

Cause: the wasm backend lowers some capability calls straight to its own
runtime (compiler/wasm rt.xo: `$env_get`, `rt.clock_now`, `$clock_sleep`;
compiler/ssa `llrt_or_none` returns None on wasm), so the native
prelude's checks of the test world (`process.env_get`'s FakeEnv,
`poll.wall_now`'s virtual clock, cabi.xo `clock_sleep`) never run. The
harness's channel and start were native only too: `testworld.write_real`
and `quit` had only Linux, Darwin and Windows arms, `twrun.start` (which
records XO_TEST from the C runtime's envp) was not called on wasm, and a
Str's leaves in the wasm runtime are (object, length) with the bytes at
object + 8.

Fix (decision 0610): in a wasm test build (`Builder.tests`), Env.get and
Env.vars go through the prelude (`llrt_any`), Clock.sleep through
`twrun.wasm_sleep`; `sysio.clock_now`'s wasm arm reads the virtual clock;
`wait_counted` moves it directly on wasm (one context, no poll timers);
compiler/ssa `wasi_served` serves `write_real`, `quit` and
`process.env_real_at` from rt.xo (`$tw_write_real` adds 8 to the Str).
When a std path checks the test world, check that the wasm backend does
not lower the call around it: grep compiler/ssa for the capability's key
(`"Env.get"`, `"Clock.sleep"`) and `b.wasm`.
