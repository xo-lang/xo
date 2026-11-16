# std/ui changes

## 2026-11-16: feature: busy state and progress bar for navigations and forms

- Kind: feature
- Commit: this push
- Found by: owner (a delay after clicking sign in on xw's dashboard looked like nothing happened)
- Go reference: none
- Test: headless Chrome ~/xo-lead/notes/stdui/e2e/drive-busy.mjs with 600 ms latency (6/6); pages 14/14, navigation 17/17, todo 21/21, morph 3/3, redeploy 6/6; internal/uiruntime and std/ui tests
- Shipped in: not yet released

While a navigation or form waits, the page carries `data-xo-busy` and `aria-busy`, a POST form's buttons are disabled (no double submit), and after 150 ms a thin bar shows at the top (`--xo-progress` sets its color).

## 2026-11-15: feature: the page layer (server pages, forms and frames without full loads)

- Kind: feature
- Commit: this push
- Found by: owner (search was a full load; std/ui should grow past one dashboard)
- Go reference: none
- Test: std/ui_test.xo (`plain` escaping and runtime script, `serve_runtime` and `runtime_url`); headless Chrome ~/xo-lead/notes/stdui/e2e/drive-pages.mjs 14/14 (plain and App pages swap both ways, search as you type with one request per burst and the URL replaced, Enter pushes, 422 in place and 303 followed, frames, shared links, pages without the runtime load normally); navigation 17/17, todo 21/21, morph 3/3, redeploy 6/6
- Shipped in: not yet released

`ui.plain`, `ui.serve_runtime` and `ui.runtime_url` make server pages that the runtime takes over (decision 0670): links, GET forms (with `data-xo-submit-on-input` for search as you type), POST forms (303 and 422) and frames (`data-xo-frame`) work without a full load.

## 2026-11-15: feature: stamped runtime and client URLs; open pages reload onto a new build; sync on socket reconnect

- Kind: feature, fix
- Commit: this push
- Found by: owner (asset caching on rebuilds, after link navigation kept tabs open across deploys); the reconnect gap while testing it
- Go reference: none
- Test: std/ui_test.xo (stamped URLs, immutable only for the current stamp, gzip and 304 for the runtime, build in sync replies); internal/uiruntime TestNewBuildReloadsAfterQueue, TestSocketOpenSyncs (fails before: no sync on open); headless Chrome redeploy under open tabs (6/6), navigation 17/17, todo 21/21, morph 3/3
- Shipped in: not yet released

`/_xo/ui.js` and `/_xo/app.wasm` are served at `?v=` content stamped URLs and cached for good; the runtime is gzipped with an ETag. A page seeing a new build (`Sync.build`) sets `data-xo-stale` and reloads once its queue is sent and nothing is being typed. A page syncs when its live socket opens, so a restart or a dropped socket no longer delays updates by up to 30 s.

## 2026-11-15: feature: links between std/ui pages without a full load

- Kind: feature
- Commit: this push
- Found by: owner (page changes without refreshing the whole page; xw's dashboard has 7 std/ui pages)
- Go reference: none
- Test: headless Chrome, ~/xo-lead/notes/stdui/e2e/drive-nav.mjs against a server with two counter pages, the todo wasm page and a plain page (17/17: switch in 4 to 11 ms with the window kept, back and forward keep state and scroll, a left page's blocked queue drains to its own endpoint and never to the next page, plain pages and data-xo-reload load normally); internal/uiruntime tests; the 21 todo and 3 morph browser checks
- Shipped in: not yet released

The runtime follows a same-origin link to another std/ui page itself (decision 0668): it fetches the page, patches it in, pushes history and starts that page's runtime; back and forward restore scroll. `data-xo-reload` opts a link out.

## 2026-11-15: perf: the view is patched in place; an idle sync no longer sends the page

- Kind: perf
- Commit: this push
- Found by: owner (selective refresh) and xw-1's dashboard numbers (an idle sync returned the whole page, 5 to 12 KB; a horizontal scroll inside a table and a text selection reset on each update)
- Go reference: none
- Test: internal/uiruntime TestMorphKeepsNodes (keyed reorder keeps nodes, the focused input keeps its text, an open details and a scrolled box survive, values, checkboxes and changed tags follow the view); std/ui_test.xo (an idle sync answers same with no html or model); headless Chrome: a page being typed in sees another tab's add within 8 ms with its text and focus kept (the old runtime never showed it: the whole view was held while an input had text), and the 21 checks of examples/ui-todo
- Shipped in: not yet released

The runtime patches the new view into the page instead of setting innerHTML, so focus, typed text, scroll positions, open details and selections survive an update, and a page being typed in still shows other updates. A sync with nothing new is about 100 bytes (`Sync.same`) instead of the whole view (2,644 bytes in examples/ui-todo).

## 2026-11-15: fix: a click during a sync is no longer lost

- Kind: fix
- Commit: this push
- Found by: xw-1 (xw's dashboard on std/ui), asked whether a link click during a replace is lost
- Go reference: none
- Test: internal/uiruntime TestReplaceWaitsForPointerUp (fails before: the view was replaced while the pointer was down)
- Shipped in: not yet released

A sync reply or prediction that arrived between pointerdown and pointerup replaced the element under the pointer, so its click never fired. The runtime now holds that view until the pointer is up and shows it after the click.

## 2026-11-15: feature: server Msgs, log compaction, sign-in redirect

- Kind: feature
- Commit: this push
- Found by: xw-1 (xw's dashboard on std/ui: its model comes from Postgres, not client Msgs)
- Go reference: none
- Test: std/ui_test.xo (a server Msg reaches a replica in order with its own pending Msg; Host.apply shows on the page and a sync naming SERVER_CLIENT is 400; a host with keep 5 holds under 10 entries after 100 Msgs and a stale page gets a snapshot; signin_url answers 303 and a sync 401)
- Shipped in: not yet released

`Host.apply(msg)` and `Server.apply(msg)` apply the server's own Msgs (`SERVER_CLIENT`, logged, live pages woken, `allow` not asked). `host(keep:)` bounds the log (default 10000 entries, compacted at twice that); before, a host's log grew without bound. `host(signin_url:)` redirects a signed-out page request.

## 2026-11-15: feature: principals, allow and redact (decision 0664, Early)

- Kind: feature
- Commit: this push
- Found by: decision 0659 stage 4
- Go reference: none
- Test: std/ui_test.xo (a client bound to its principal; a refused Msg acknowledged, counted, logged as refused and not applied; 401 and 403 from the host; redact on the page and every sync with no log entries; 5 replicas converge with refusals over a lossy network, 40 seeds with and without compaction); internal/uiruntime (401 keeps the queue, 403 drops it and reloads); the 21 headless Chrome checks of examples/ui-todo
- Shipped in: not yet released

`ui.host(app, auth:, allow:, redact:)`: `auth` gives each request a principal (401 without one) and a client id stays with its first principal (403 otherwise); `allow(model, principal, msg)` refuses a Msg before update (acknowledged, logged as a refused entry, `Down.refused`); `redact(model, principal)` shapes everything a principal receives. `Server.receive_as`, `Entry.refused`; `Server.version()` counts refused entries too.

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
