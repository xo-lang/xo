# 0688: wasm library builds are WASI reactors with `as c` exports

Status: Accepted (owner, 2026-11-16, as written; proposed by Xo Lang 1, 2026-11-16, task #53)

## Context

std/ui stage 3 (decision 0659) runs an App's client program, built for
wasip1/wasm, in the browser on every event: a new instance per event,
JSON on stdin, `_start`, JSON on stdout. The wasm backend exports only
`_start`, `memory`, `xo_live` and `xo_total` (compiler/wasm/rt.xo,
gen.xo). Measured in headless Chrome
(bench/results/ui-wasm-client-2026-11-15.md): p95 0.4 ms a step, 2.6 ms
with the CPU slowed 6x. Not a blocker; exports buy one live instance
(heap and model kept between events, no instantiation per event) and no
stdio round trip. Decision 0574 (C library exports, `pub fn NAME(...)
as c "sym"`, `xo build --lib`) lists wasm as "a separate decision"; this
is it, for the core module (wasip1). The component path (decision 0447)
already exports wasi:http.

## Decision

1. **`xo build --lib=shared --targets=wasip1/wasm` writes `NAME.wasm`,
   a WASI reactor**: no `_start`; an `_initialize` export that runs the
   runtime's and the packages' initialization once; one export per
   `as c "sym"` function, named `sym`. The host calls `_initialize`, then
   any export any number of times on one instance. `--lib=static` is an
   error for wasm (a core module has no archive form).
2. **Boundary types** follow 0574's table, with wasm's widths: Int is
   i64, Float f64, a handle i64. A Str or Bytes parameter is two i32s
   (pointer and length into linear memory), written by the host into a
   block from the export `xo_alloc(len: i32) -> i32` and released by the
   callee's return (borrowed for the call, as 0574's). Unlike C exports,
   a Str or Bytes **result** is allowed on wasm: the export returns the
   pointer (i32) and the length is read from `xo_result_len() -> i32`;
   the block stays valid until the next export call. (A packed i64
   would reach JS as a BigInt; two calls keep JS plain numbers.)
3. **Faults**: the fault's report goes to stderr, then the export traps
   (`unreachable`). The instance is dead; the host makes a new one
   (std/ui's runtime falls back to a fresh instance or its thin client).
   An error result (`-> T ! E`) returns as 0574's: the value, with the
   error readable through `xo_last_error`.
4. **Output and leaks**: stdout and stderr are flushed after each export
   call returns. The end-of-program leak check and exit flush do not run
   (a reactor never exits); `xo_live` and `xo_total` stay exported for
   the host to watch.
5. XO1518's rules apply, with item 2's Str/Bytes results; a library
   with no `as c` function is an error, as 0574's `--lib` on a package
   with `fn main`.

## Consequences

- std/ui's client package declares
  `pub fn xo_ui_predict(input: Str) -> Str as c "xo_ui_predict"`
  calling `ui.predict`, built with `--lib=shared`; the JS runtime keeps
  one instance per page. Measured against the per-event baseline with
  ~/xo-lead/notes/stdui/wasm1/drive-bench.mjs before std/ui switches.
- Tests: a golden-style test instantiates a reactor in the Go test
  wasm host (wasmtime, as internal/selfhost's wasm tests; `--invoke`) and calls an Int, a Str-in/Str-out and a
  faulting export; the leak counters stay flat over 1000 calls.

## Alternatives

- **Keep the per-event instance**: works today; pays instantiation and
  stdio per event.
- **Component exports (wasip2) in the browser**: needs a component
  runtime (jco) in the page; the core module needs none.
- **A Str result as a packed i64**: one call, but a BigInt in JS.
- **Exports in a program build (with `_start`)**: a program that also
  exports would run main and exit through proc_exit; a reactor is the
  WASI form for a module whose host calls in.
