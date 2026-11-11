# 0610: `xo test --backend=wasm` runs std's tests in WASI test modules

Status: Accepted (owner, 2026-11-11). Was: Proposed (Xo Lang 2, 2026-11-11; owner-approved stability item "run
the std corpora on wasm", relayed by the lead)

## Context

The wasm backend built and ran programs (the playground, the wasm goldens of
internal/selfhost), but std's own tests never ran on it: `xo test
--backend=wasm` built a test module whose every test faulted at its first
report (`testworld.write_real` had only Linux, Darwin and Windows arms), and
the module could not see XO_TEST (the harness's variables come from the C
runtime's envp, which a WASI module does not have). Bugs that only the wasm
backend shows (gotchas/native-layout-reads-break-wasm.md has three) were
found by programs, not by std's tests.

## Decision

1. `xo test --backend=wasm` builds the tests as a wasip1 module and runs each
   test in its own process under the runner `XO_TEST_EXEC` names (required;
   `node compiler/wasm/run.mjs` in the repository), as `go test -exec`.
   No batches and no replays over a pipe (`TestOpts.single`): the WASI
   module's main runs XO_TEST's test and exits (compiler/ssa testmode.xo has
   no `llrt_test_next` loop in a WASI module).
2. The harness's channel and start are served by the wasm runtime:
   compiler/ssa `wasi_served` maps `testworld.write_real` and `quit` to
   rt.xo `$tw_write_real` (fd_write on fd 1) and `$tw_quit` (proc_exit), and
   `process.env_real_at` to `$env_real_at` (environ_get, the strings kept
   for the run, as envp's); the test main calls `tw_start` on wasm too, and
   `twrun.start` skips its function addresses there.
3. The test world is the native one: in a wasm test build Env.get and
   Env.vars go through the prelude (FakeEnv), Clock.sleep and
   sleep_or_cancel through `twrun.wasm_sleep`, `sysio.clock_now` reads the
   virtual clock, and `wait_counted` moves it directly (one context, no
   poll timers). Plain wasm programs keep their direct runtime calls.
4. What WASI preview 1 cannot run is a listed skip with its reason
   (compiler/xo wasmskip.xo), shown in the summary line (decision 0437),
   never a pass and never a build failure: the modules that need sockets
   (amqp, amqptest, native/http, native/llrt/netconn, native/llrt/pg), SQLite
   (C), the native allocator's own tests (heapos, obj, libcmem), and two
   tests of native layout (raw's Str view, ctx's services record). A module
   skip returns before lowering; a test skip whose body wasm cannot lower
   keeps a `target.arch` guard in the test so the module builds.

## Consequences

- First full run, 2026-11-11 (host 2, linux/arm64, Node 20): all 65 std test
  modules pass on wasm: 594 tests pass, 0 fail, 109 skipped (108 listed
  wasm skips, 1 `uses` test without --real);
  ~/xo-lead/notes/xo-lang-2/wasm-sweep-2026-11-11b.txt. The first run before
  the test world fixes: 51 of 65 modules passed, 5 tests failed, 11 modules
  did not build.
- internal/selfhost TestWasmStdTests runs a sample (base32, native/time,
  uuid, native/prelude, amqp's skips, raw's skip).
- One process per test and per property case: slower than a native batch
  (base32's 108 processes take about 10 s under Node), fine for a tier.
- A test world wait that needs another context (a task woken by a virtual
  timer) has no wasm scheduler support yet: the one-context `wait_counted`
  moves the clock without firing poll timers. No std test needs it today.

## Alternatives

- Batches on wasm (the native `llrt_test_next` loop): needs the test world's
  reset between runs in the wasm runtime; one process per test works now.
- Skips written in each test (`if target.arch == Wasm32 { return }`): such a
  test reads as a pass; the harness list names the reason instead.
- Porting the native allocator and Str view tests to wasm's runtime: they
  test code a WASI module does not contain; the wasm runtime's own tests
  (the selfhost wasm goldens) cover its allocator.
