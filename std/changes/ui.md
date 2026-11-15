# std/ui changes

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
