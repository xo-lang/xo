# std/ui against a hand written front end: one todo app built both ways (2026-11-20)

Task #499. One app, examples/ui-todo, served three ways from the same
Model and the same `todo.update`:

- **ui**: std/ui with its wasm client (`examples/ui-todo/server`): the
  page is rendered on the server, and the browser runs `update` and
  `view` itself to predict each Msg (decisions 0659, 0663).
- **thin**: the same server with `thin` as its second argument: std/ui's
  runtime JavaScript only; every Msg waits for the server's view.
- **split**: the traditional design (`examples/ui-todo/split`): a
  static HTML, CSS and JavaScript front end written by hand, embedded
  with `embed("static")` and served by `http.file_server`, plus a JSON
  API (`GET /api/state`, `POST /api/msg`) and server-sent events
  (`GET /api/events`). The browser renders the Model itself.

Tree: d2dbc9517 plus the examples and driver in this commit. xo: the
cached build of d2dbc9517. Host: the Mac Studio (darwin/arm64),
headless Chrome at 390 by 844, cache off, server and browser on
127.0.0.1. Driver: `bench/ui-vs-split/drive.mjs` (30 adds per site,
600 ms apart, so each add starts with no request in flight; then 30
more with 100 ms of latency added to every request through
`Network.emulateNetworkConditions`). Three rounds; raw output in
`bench/ui-vs-split/raw/` (`roundN-SITE.json`, and `roundN-SITE-server.json`
for the server's CPU seconds and RSS over the drive).

## Code

Lines (`wc -l`, comments included) beyond what both share. Both share
`todo.xo`'s Model, Msg, `update` and `initial` (lines 1 to 53 and 82 to
85).

| | ui and thin | split |
|---|---|---|
| Rendering | `view` 28 lines of Xo, `app` 5 | `static/app.js` 57 lines of JavaScript, `static/index.html` 21 lines of HTML |
| Styles | `CSS` 19 lines (a Xo string) | `static/style.css` 16 lines |
| Server | `server/main.xo` 23 | `split/main.xo` 91 (the API, the event stream, a subscriber map) |
| Client build | `client/main.xo` 9 | none |
| Total | 84 lines, one language | 185 lines, four languages |

The split side's `app.js` repeats `view` by hand: the Msg's JSON form
(`{"Add":{"title":...}}`, the enum encoding std/json gives `todo.Msg`)
and the Model's field names are written twice and checked by nothing.
A renamed field in `todo.Model` is a compile error for ui and thin and
a blank list for split.

## Results

| | ui | thin | split |
|---|---|---|---|
| Server binary (bytes) | 6,682,110 (the 205 KB wasm client embedded) | same binary | 4,917,601 |
| Cold load, bytes on the wire | 106,025 in 6 requests | 11,685 in 4 | 4,410 in 5 |
| of which: page HTML | 2,415 | 2,231 | 807 |
| of which: script | ui.js 9,059; app.wasm 93,863 | ui.js 9,059 | app.js 2,297; style.css 1,130 |
| Todos in the HTML before any script runs | yes | yes | no (an empty `<ul>`) |
| Add, until the todo is in the page, p50 / p95 ms, no added latency | 1.3 to 1.6 / 4.0 to 9.9 | 1.9 to 2.4 / 2.9 to 5.5 | 1.7 to 2.3 / 2.7 to 71.3 |
| Add, p50 / p95 ms, 100 ms added latency | 1.5 to 1.9 / 2.4 to 3.3 | 108.7 to 109.7 / 112.2 to 113.2 | 107.9 to 110.8 / 111.9 to 220.2 |
| Server CPU over a drive (60 adds, three page loads) | 0.03 to 0.04 s | 0.03 to 0.04 s | 0.01 to 0.02 s |

Ranges are the three rounds' lowest and highest. Bytes are Chrome's
`encodedDataLength` (headers included): std/ui gzips `ui.js` and the
wasm module; `http.file_server` sent split's files uncompressed (gzip
-9 would make them 377, 535 and 1,072 bytes). First contentful paint
ranged 28 to 264 ms headless with no order between the sites, so it is
not reported.

## Findings

1. **Latency: the wasm client does not wait for the network.** At 100 ms
   added latency an add shows in 2 ms on ui and in one round trip (about
   109 ms) on thin and on split. Thin and split are the same design for
   latency: both wait for the server's answer.
2. **Bytes: split is the smallest first load, ui the largest.** split
   loads 4.4 KB, thin 11.7 KB, ui 106 KB, 94 KB of it the wasm module.
   The module buys the latency in finding 1; thin is the choice when
   first load matters more.
3. **Content without scripts: only std/ui.** ui and thin send the
   rendered todos in the page HTML (search engines, no-JavaScript
   readers, a first paint before scripts load). split's HTML is an empty
   list until `app.js` fetches the state.
4. **Code: one language against four.** The split version is 2.2 times
   the lines and holds the second copy of the view and the wire format
   that nothing checks against the server's types.
5. **One std/ui inefficiency found.** Adds sent back to back (no 600 ms
   gap) took 215 ms on thin at 100 ms latency, two round trips: each
   action makes a second, redundant sync, because the live channel's
   notice for the change arrives while the change's own sync is in
   flight and queues another (runtime `ws.onmessage` sets `again` when
   `busy`). A trace of one add showed 3 sequential `POST /_xo/sync`. The
   first sync's reply already carries the new view (checked with curl),
   so a lone add is one round trip, as in the table. Task #500.
6. **Server CPU is not a differentiator at one viewer.** All three spent
   10 to 40 ms over a drive. std/ui's per-viewer costs at scale are
   measured in commit 502a0170c (1,000 viewers).
