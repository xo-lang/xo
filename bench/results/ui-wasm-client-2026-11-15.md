# std/ui stage 3: the app's update and view as wasm in the browser (2026-11-15)

Decision 0659 stage 3 runs an App's `update` and `view` in the browser,
so a click shows at once instead of after a round trip. This experiment
asks whether Xo's existing wasm backend is good enough for that today,
with no compiler change.

## Setup

- xo at origin/main 24c9a9173 plus nothing (worktree `ui-wasm-exp`).
- The program: examples/ui-todo's Model, Msg, `update` and `view`, with a
  `main` that reads the model's JSON and a Msg's JSON on stdin and writes
  the new model's JSON and the rendered HTML (`ui.render(view(m))`).
  Built with `xo build --targets=wasip1/wasm`: a WASI command module.
- The wasm backend exports only `_start` and `memory` (plus `xo_live`,
  `xo_total`), so each step instantiates the compiled module, runs
  `_start` once, and reads stdout. That is the worst case: no exports, a
  JSON round trip and a fresh instance per click.
- In the browser, a 40 line WASI shim (stdin from a buffer, stdout
  collected, `proc_exit`, clock, random; every other import ENOSYS) and
  `root.innerHTML = html`. Headless Chrome over CDP at 390 by 844,
  `Emulation.setCPUThrottlingRate` 1, 4 and 6 for a fast, a mid and a low
  phone. Scripts in `~/xo-lead/notes/stdui/wasm1/` on the Mac Studio
  (run.mjs, shim.js, drive-bench.mjs, parity.sh).
- Steps: 50 adds then 25 toggles (75 steps), ending with 50 todos and
  about 13.6 KB of HTML.

## Results

| | Value |
|---|---|
| Module size | 205,092 bytes; 68,757 gzip -9; 54,935 brotli -q 11 |
| Imports | 23 WASI preview1 functions; the program calls 7 |
| Parity with the native build | 11 steps byte-identical (model JSON and HTML), including HTML escaping, Unicode, an empty title refused, a fourth seat refused |
| Node 22 WASI, a step with file stdin/stdout | p50 1.84 ms, p95 3.65 ms, first 44 ms |

Chrome (a step is instantiate plus `_start`; DOM is `innerHTML` plus a
layout read):

| CPU | compile once | step p50 / p95 / max | DOM p50 / p95 |
|---|---|---|---|
| 1x (Mac Studio) | 8 ms | 0.2 / 0.4 / 2.4 ms | 0.4 / 0.8 ms |
| 4x slower | 28.6 ms | 0.9 / 1.5 / 10.2 ms | 1.9 / 4.0 ms |
| 6x slower | 42 ms | 1.8 / 2.6 / 15.9 ms | 3.7 / 6.0 ms |

## Findings

1. **The existing wasm backend is enough for stage 3.** Even with the CPU
   slowed 6x, a step plus the DOM update is under 9 ms at p95, inside one
   16 ms frame, while instantiating per click. Exports and a resident
   instance are an optimization, not a requirement.
2. **55 KB brotli** for the app, std/json and std/ui's view code. std/ui
   imports std/http for the host, and that did not stop the wasm build or
   visibly bloat it (lazy bodies, decision 0247).
3. **Same answer as the server.** The wasm build and the native build
   give byte-identical models and HTML for the same steps, which is what
   prediction needs (with decision 0662 making `update` deterministic).
4. **Shipping it:** `embed("path")` (decision 0304) lets the server binary
   carry the client module, so an app is two builds: the client for
   wasip1/wasm, then the server embedding it.

## Next

- Stage 3 build: a client program that runs `Replica.model()` (the
  confirmed model with the pending Msgs folded over it, confirmed Msgs
  left out) and `view`, so one wasm run per render predicts; the browser
  runtime calls it on each event and still syncs as in stage 1. Then the
  14 browser checks plus "a click shows within 16 ms" offline.
- Later: wasm exports (decision 0574's `as c "name"` on the wasm
  backend) to keep one instance and skip stdin/stdout; measure against
  this baseline.

## Follow-up (2026-11-15, same day)

The stage 3 build landed on this baseline: `ui.client_main` and
`ui.predict`, `host(client: ...)` serving `_xo/app.wasm`, and the runtime
running it per event (examples/ui-todo client/ and server/). Headless
Chrome, two tabs at 390 px: an offline add shown in 1.5 ms and a toggle
in 1.1 ms from the event, an offline reserve shown as waiting, 21/21
checks. Those checks also found two tabs sharing one client id
(gotchas/ui-tabs-share-localstorage.md).
