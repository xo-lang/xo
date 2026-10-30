# WebAssembly runtimes and JIT: the owner's Q&A of 2026-10-30

Background and recommendations from the owner's questions on 2026-10-30
(evening), kept for later. The decisions it led to are in
[decision 0405](decisions/0405-xo-wasi.md) (Runtimes section). Numbers are
from that ADR and the commits named; anything marked "expected" is not
measured.

## What `xo.wasm` is

- `xo build --targets=wasip1/wasm compiler/xo` builds the toolchain itself
  as a WASI preview1 module with Xo's own wasm backend (compiler/wasm), not
  the Go backend. It runs `check`, `fmt`, `vet` and `build` for every
  native target; commands that need processes or sockets report XO1513.
- Its use: running xo where native xo is not shipped or not allowed (a
  browser playground, sandboxed agent environments, an OS or CPU without a
  native build).
- State on 2026-10-30: scripts/wasi-smoke.sh passes (4e385b8ba; 71 s under
  Node with `--liftoff-only`); the module is about 160 to 184 MB, about 1.6
  times the native binary, with one 12.4 MB function. Target: 0.36.0 if
  its release checks pass (decision 0405).

## Runtimes: why there is no "pick a V8 compiler" choice

WebAssembly is a portable bytecode: any wasm runtime runs it, and making a
`.wasm` from Xo, Go or Rust source involves no runtime at all.

| Runtime | Engine | Notes |
|---|---|---|
| Chrome, Edge, Node, Deno | V8 | Liftoff (fast baseline compiler) then TurboFan (optimizing, tiers up hot functions in the background) |
| Firefox | SpiderMonkey | |
| Safari | JavaScriptCore | |
| wasmtime | Cranelift | the reference WASI implementation; `wasmtime compile` writes machine code once (AOT) |
| Wasmer, WasmEdge | own | standalone |
| wazero | own, pure Go | a Go library or `wazero run`; no cgo, no install beyond Go |

- Go: `GOOS=wasip1` runs on standalone runtimes; `GOOS=js` needs a
  JavaScript host and `wasm_exec.js`. Rust: `wasm32-wasip1` for WASI,
  `wasm32-unknown-unknown` with wasm-bindgen for browsers. Xo's
  `wasip1/wasm` is the WASI kind.
- `--liftoff-only` is a Node/V8 workaround, not a design choice: V8's
  TurboFan tier-up of xo's huge module ran away on memory (f7d7f422c: the
  runaway was the tier-up, not the wasm heap). run.mjs (the Node launcher
  shipped with `xo.wasm`) sets it by default for large modules;
  `XO_WASM_V8_FLAGS` overrides it. Liftoff code runs slower than TurboFan
  code (not measured for xo). Typical Go or Rust wasm modules are far
  smaller and do not hit this.
- The lasting fix is the module: split the 12.4 MB function (over V8's
  per-function cap) and shrink the module, so every runtime, a browser's
  V8 included, takes it without flags.

## Recommendation agreed with the owner

1. Target WASI preview1, not an engine: any WASI runtime is supported.
2. Test wasmtime first (reference implementation, AOT compile once) and
   Node second (most machines have it, through run.mjs).
3. Do not build our own runtime: a runtime is a native program shipped per
   platform, and where we ship a native binary, native xo is already
   faster; it would remove the reason `xo.wasm` exists.
4. After 0.36.0: a Go test that runs `xo.wasm` under wazero (version,
   check, one build), so the main checks cover WASI with only Go
   installed; list wazero as supported if it handles the module.

## wazero

- A WebAssembly runtime written in pure Go; a Go program imports it and
  runs a module in-process. It runs any wasm module, from any compiler; it
  has no tie to Xo's Go backend.
- Gains for Xo: WASI coverage from plain `go test` in our Go test
  infrastructure (internal/xotool, cmd/xo); embedding the Xo compiler in a
  Go service (a checker or playground server) with no processes; `go
  install` on any platform Go supports; WASI preview1, which `xo.wasm`
  targets.
- Costs: its compiler is simpler than wasmtime's, so `xo.wasm` is expected
  to run slower; its handling of a 184 MB module with a 12.4 MB function is
  untested.

## A wasm runtime written in Xo (a rewrite of wazero)

- No gain for `xo.wasm`: a runtime in Xo is a native binary, and wherever
  it runs, native xo runs.
- Real gains elsewhere: Xo programs could host sandboxed wasm plugins or
  user scripts (a backend feature Go programs get from wazero); a wasm to
  native compiler could lower to Xo's existing arm64 and amd64 encoders; a
  large real Xo program that exercises the compiler and runtime.
- Costs: a big project (compiler, WASI layer, spec conformance tests); a
  JIT needs writable and executable memory (`MAP_JIT` on macOS), which
  Xo's runtime does not expose today.
- If wanted: an ADR for "wasm plugins in Xo", an interpreter first, then
  AOT through our encoders (no executable memory needed). Not before
  0.36.0.

## AOT and JIT

- Ahead of time (AOT): machine code is made before the program runs and
  saved. Native Xo (`xo build`), Go and Rust are AOT. For wasm,
  `wasmtime compile xo.wasm` writes `xo.cwasm` once, using the slow
  thorough optimizer since it is paid once; later runs start at once.
- Just in time (JIT): the runtime compiles while the program starts or
  runs, every run. V8 (Node, Chrome) is a JIT: Liftoff at start, TurboFan
  for hot code. For a 184 MB module that is seconds before xo does
  anything, and it is where V8's memory ran away.
- `xo.wasm` adds a second step to Xo's AOT build: Xo to wasm, then wasm to
  machine code on the user's machine (AOT with wasmtime, JIT with V8).

## Should Xo have a JIT?

Recommendation (owner agreed, 2026-10-30): no general JIT. AOT native
code is the right default for a statically typed backend language, as for
Go and Rust.

- What a JIT is good for: running code with no build step; optimizing from
  run-time behavior (mostly a gain for dynamic languages; Xo knows its
  types, and decision 0403's profile-guided layout gets some of it AOT);
  running code that arrives at run time (plugins, eval, hosted wasm).
- Where it could serve Xo: a REPL, notebooks or a playground; hot reload of
  a changed function in a running server during development; hosting wasm
  plugins at native speed; the debugger evaluating an expression that calls
  functions.
- Against a general JIT:
  - The edit loop is already fast without one: an HTTP edit about 14 ms on
    arm64 (911b5e65b), an unchanged test rerun 8.4 ms with the test result
    cache (decision 0439), and the compile server (decision 0283) targets
    what is left.
  - Platform limits: writable and executable memory needs `MAP_JIT` and
    entitlements on macOS, is forbidden on iOS, and is blocked by some
    hardened Linux setups.
  - It works against Xo's design for agents: reproducible, byte-identical
    builds and cross compilation are AOT strengths; JIT code differs run to
    run and is harder to profile, debug and audit.
  - Production servers want predictable performance: no warm-up, no
    compiler inside the server.
- If interactive use or hot reload becomes a goal: a narrow,
  development-only JIT on the existing arm64 and amd64 encoders, with its
  own ADR.

## Direction (owner, 2026-10-30 evening)

The owner's aim: Xo should be great and fit uses that are or could be
popular, rather than serve one named audience for `xo.wasm`. So:

1. 0.36.0: ship `xo.wasm` (decision 0405's release checks).
2. Right after: split the 12.4 MB function (it helps every runtime: a
   browser playground on V8 and sandboxed agents on wasmtime alike) and add
   the wazero Go test.
3. Xo programs as wasm, where wasm is popular for backends: edge and
   serverless functions (Cloudflare Workers, Fastly Compute, Fermyon Spin),
   plugins in other systems (Envoy proxy-wasm, databases, Extism) and
   browser apps. Xo compiles programs to `wasip1/wasm` today; those
   platforms are moving to WASI preview 2 and the component model
   (`wasip2`: `wasi:http` handlers, sockets, typed interfaces). A research
   ADR comes first: what `wasip2` and `wasi:http` take for Xo programs,
   which platforms they open, and a `wasi:http` hello-world handler as the
   first measured step; nothing is built before the owner decides.
