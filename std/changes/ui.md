# std/ui changes

## 2026-11-15: perf: the client module loads in about half the time

- Kind: perf
- Commit: this commit
- Found by: decision 0659 stage 3's time to resume (headless Chrome, 390 px)
- Go reference: none
- Test: std/ui_test.xo (gzipped when accepted, ETag and 304, accepts_gzip tokens and q=0, the preload link); headless Chrome, examples/ui-todo, median of 3 cold loads: prediction ready at 721 ms on slow 4G (1,566 ms before) and 164 ms on 4G (331 ms), the 21 browser checks pass
- Shipped in: not yet released

The client module (203 KB for ui-todo) is served gzipped (69 KB) with an ETag, so a repeat visit gets a 304; the page preloads it from its head, and the runtime compiles it while it streams in.

## 2026-11-15: fix: two tabs of one browser lost each other's Msgs (Early)

- Kind: fix
- Commit: this push
- Found by: decision 0659 stage 3 browser checks (a second tab's toggle never reached the server)
- Go reference: none
- Test: internal/uiruntime TestTwoTabsAreTwoClients (the runtime under Node as two tabs sharing localStorage; failed before: the server applied a1, b1, b2 of a1, b1, a2, b2), TestClosedTabQueueIsAdopted
- Shipped in: not yet released

Tabs shared one client id and seq counter in localStorage, so once a second tab opened, Msgs the two sent with the same seq were dropped as duplicates. Each tab is now its own client (its id in sessionStorage, its queue in localStorage under that id); a new tab adopts the queue of a tab that closed, so offline work still survives closing the tab.

## 2026-11-15: feature: the browser predicts with the App's own wasm (Early)

- Kind: feature
- Commit: this push
- Found by: decision 0659 stage 3
- Go reference: none
- Test: std/ui_test.xo (predict folds pending Msgs and renders what the server renders once it has them; confirmed Msgs left out and counted; the wasm route and page attribute; a thin host; `?model=1`); headless Chrome over CDP with examples/ui-todo built for wasip1/wasm: an offline add shown in 1.5 ms and a toggle in 1.1 ms, an offline reserve shown as waiting, 21/21 checks
- Shipped in: not yet released

`ui.client_main(os.stdio, app)` is the main of an App's client program, built with `--targets=wasip1/wasm`; `ui.host(app, client: bytes)` serves it at `_xo/app.wasm`, and the runtime runs it on each event to show the predicted page at once (`ui.predict`: pending Msgs over the server's model, confirmed ones left out; `data-xo-awaiting` counts them). `Sync` gains `model` (sent for `?model=1`). Without a client the runtime is the thin client it was.

## 2026-11-15: feature: confirmed Msgs (Early)

- Kind: feature
- Commit: this push
- Found by: decision 0659 stage 2
- Go reference: none
- Test: std/ui_test.xo (a confirmed Msg is not predicted and is listed as awaiting until confirmed; four clients reserving three seats offline never show a seat the server refused)
- Shipped in: not yet released

`App.confirm` marks Msgs that wait for the server; `Replica.awaiting()` lists them. `ui.app(init, update, view)` builds an App (confirm false for every Msg) and `.confirming(f)` sets it. Breaking for an `App{...}` literal, which now needs `confirm`; use `ui.app`.

## 2026-11-15: feature: live updates over a websocket (Early)

- Kind: feature
- Commit: this push
- Found by: decision 0659 stage 1 (the ADR names std/websocket for the sync channel; 1b polled every 2 s)
- Go reference: none
- Test: std/ui_test.xo (a changing sync notifies a live subscriber, one that changes nothing does not; the page names the endpoint); headless Chrome over CDP, two clients: another tab saw each add in 1 to 10 ms (about 1.7 s by polling), a background tab a toggle in 110 ms, a reconnect's queued work in 102 ms (1,147 ms); 14/14 checks
- Shipped in: not yet released

`mount` adds `prefix/_xo/live`, a websocket that sends the server's version when it changes; the runtime syncs when it hears of a newer one, polls every 30 s while the socket is up (2 s while it is down), and reconnects with backoff. `Host.live()` counts the connections.

## 2026-11-15: feature: serving an App over HTTP (Early)

- Kind: feature
- Commit: this push
- Found by: decision 0659 stage 1, step 1b
- Go reference: none
- Test: std/ui_test.xo (the host's routes, a prefix, refused content types and bodies, field placeholders); examples/ui-todo (its update and a browser's sync), checked in headless Chrome over CDP (two clients, offline queueing and reconnect, server-decided seats, reload)
- Shipped in: not yet released

`ui.host(app, title, css)` and `mount(router, prefix)`: the first page, the browser runtime and a JSON sync endpoint over a Server behind a Mutex; `ui.field(name)` for form values in a Msg; `page` takes `css` and `sync_url`. The runtime is thin until stage 3: it queues Msgs in localStorage, syncs on events, every 2 s, on becoming visible and on reconnecting, and shows the server's view.

## 2026-11-15: feature: std/ui, local-first UI core (Early)

- Kind: feature
- Commit: this push
- Found by: decision 0659 stage 1 (owner, 2026-11-15)
- Go reference: none (Xo's own design; the reconciliation follows the experiment in bench/results/ui-local-first-2026-11-15.md)
- Test: std/ui_test.xo: rendering and escaping, page state escaping, prediction and confirmation, duplicates and gaps, an overturned prediction, snapshot after compaction, convergence over a lossy, reordering network (with and without compaction, and as a property)
- Shipped in: not yet released

New module, Early: `App` (init, a pure update, a view), `Html` views rendered with escaping and no raw HTML, `on` events as JSON data attributes, `page` (the rendered view plus the state for the client runtime), and the reconciliation protocol as pure values: `Server` (applies each client's Msgs once and in order, a log with acks, batches and compaction with snapshots) and `Replica` (predicts at once, replays the log, folds pending Msgs on top). Transport and the browser runtime come in later steps.
