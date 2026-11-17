# std/ui changes

## 2026-11-17: perf: pg_bus sends NOTIFYs from a task, batched

- Kind: perf
- Commit: this push
- Found by: task #186 (users over several instances, decision 0686 plan step 4)
- Go reference: none
- Test: std/ui "pg_bus queues the newest version per scope and sends up to 100 NOTIFYs a query"; "a burst of pg_bus publishes over many scopes reaches the other instance, the last version of each"
- Timing: see decision 0686 "Users over several instances (2026-11-17)"
- Shipped in: not yet released

`pg_bus`'s publish no longer waits a round trip for its NOTIFY: it tells
the instance's own subscribers, queues the scope's newest version, and a
task sends what queued, up to 100 NOTIFYs a query.

## 2026-11-17: fix: sql_store starts when another instance creates its tables at the same moment

- Kind: fix
- Commit: this push
- Found by: task #186 (four instances started together on Postgres)
- Go reference: none
- Test: std/ui "sql_store starts when another instance created its tables at the same moment (Postgres 42710, 42P07)"
- Shipped in: not yet released

Postgres's CREATE TABLE IF NOT EXISTS can fail for the second of two
racing sessions with "type ... already exists" (42710) or "relation ...
already exists" (42P07); sql_store took that as an error and the instance
did not start. Both now mean the table is there.

## 2026-11-17: feature: real-time rooms (decision 0695)

- Kind: feature
- Commit: this push
- Found by: task #186 (std/evidence/ui/features.md, decision 0686 plan step 7)
- Go reference: none
- Test: std/ui "a room frame carries a view as a delta from the last, and reads back to it", "a room steps on its tick, sends each player its view and ack, and starts again from its snapshot (decision 0695)"
- Timing: examples/ui-arena on the Mac Studio, 20 Hz: 4,000 players at 25.7 ms p50 and 50.3 ms p99 input to frame, no late ticks; 60 Hz: 1,000 players at 8.8 ms and 17.2 ms (decision 0695)
- Shipped in: not yet released

`ui.rooms(ui.game(init, step, view), store, tasks, clock, hz:)` runs
rooms of a game on a fixed tick, with the state in memory and a snapshot
in the store every `snapshot_ms`; each player gets binary frames of its
view (a delta from the last when smaller) over a websocket mounted at
`/_xo/room`, and `/_xo/room.js` is the browser client.

## 2026-11-17: feature: commands, server side effects a Msg asks for (decision 0692)

- Kind: feature
- Commit: this push
- Found by: owner track (decision 0664 stage 4b, task #186 feature audit)
- Go reference: none
- Test: std/ui "a command runs once for each Msg the host commits, planned from the model before it, and a refused Msg plans none"; "two Hosts on one store run each command once, after a lost race too"
- Timing: none (a Host without commands is unchanged; with them, update runs once more over each committed batch)
- Shipped in: not yet released

`host(..., commands: ui.commands(tasks, of, run))` plans commands for each
Msg the Host commits (`of(model before, msg)`), runs each once in a task
on the instance that committed it, and applies a command's answer as a
server Msg. Refused Msgs plan none; an instance that loses an append race
does not run the winner's commands. At most once.

## 2026-11-17: feature: query subscriptions (host(subset:))

- Kind: feature
- Commit: this push
- Found by: task #186 (std/evidence/ui/features.md item 75; decision 0694)
- Go reference: none
- Test: std/ui "a subset sends each page only the part of the model its query subscribes to (decision 0659 point 5)", "subset applies after redact: a principal's page holds its own part of its query"
- Timing: none (a host without subset sends and does what it did)
- Shipped in: not yet released

`host(subset: fn(M, url.Values) -> M)` sends each page only the part of
the model its query asks for; a change to another part wakes no live
page and answers its syncs `same`.

## 2026-11-17: feature: the wasm client predicts user acts

- Kind: feature
- Commit: this push
- Found by: task #186 (decision 0672 Plan step 4: an offline page did not show its own cart adds in Chrome)
- Go reference: none
- Test: std/ui "predict folds the principal's pending acts over their state, so an offline page shows them (decision 0672)"; examples/ui-shop shop_test "user state: ..."
- Timing: none (one fold over the unconfirmed acts per prediction)
- Shipped in: not yet released

`ClientIn.upending` carries the principal's unconfirmed acts, and
`predict` folds the user update over their state and applies the Msgs it
emits, so a user act shows at once and an offline page shows its own.

## 2026-11-17: feature: user state across devices (App.with_user, on_user)

- Kind: feature
- Commit: this push
- Found by: task #186 (std/evidence/ui/features.md item 91; decision 0672 part 2)
- Go reference: none
- Test: std/ui "a principal's own state is the same on each of its devices and no one else's (decision 0672)", "a principal's act emits shared Msgs as that principal, through allow (decision 0672)", "on_user renders the act as JSON in a data-xo-u attribute"; internal/uiruntime TestUserActsSyncApart
- Timing: none (an App without user state sends and does what it did)
- Shipped in: not yet released

`App.with_user(init, update, view)` keeps a state per signed in
principal, synced to each of its devices through the sync's `user`
part, and `on_user(event, act)` sends an act to it. An act may emit
shared Msgs, applied as the principal's through `allow`. Each
principal's state is its own scope in the Host's store, so a change
wakes only that principal's live pages. A Host with user state and no
`auth` faults.

## 2026-11-16: feature: leases route a hot scope's syncs to one instance (decision 0686)

- Kind: feature
- Commit: this push
- Found by: owner track (leases for hot scopes, decision 0686 item 6)
- Go reference: none
- Test: std/ui "an instance that loses a race takes a lease, and the others forward that scope's syncs to it"; the racing-Scopes and group commit tests
- Timing: localstack, 4 instances x 160 appends to one scope, slowest instance: 8 clients per instance 1.44 s leased against 1.59 s racing (medians of 5), 32 clients 0.56 s against 0.68 s (medians of 3); real S3 (ca-central-1), 4 x 100 with one client each, 20.9 s against 23.5 s; every item kept once in every run (~/xo-lead/notes/stdui/s3store)
- Shipped in: not yet released

`host_with(..., lease: ui.Lease{me, forward, now_ms})` lets Hosts on one
store share a hot scope: an instance that loses an append race takes a
lease kept in the store, and the others forward that scope's syncs to it
while the lease lasts (`term_ms`, 10 s by default). Appends stay
conditional, so an instance that has not seen a lease, or cannot reach
its holder, still writes correctly. Off unless `lease:` is given.

## 2026-11-16: feature: the polling bus (Host.poll, Host.poll_every)

- Kind: feature
- Commit: this push
- Found by: task #186 (std/ui feature audit, decision 0686 plan step 5)
- Go reference: none
- Test: std/ui "a Host that polls its store hears another instance's append and wakes its live pages (decision 0686)"; "poll_every wakes a live page within its interval"
- Shipped in: not yet released

## 2026-11-16: fix: a subscriber that joins pg_bus after it listens hears -1

- Kind: fix
- Commit: this push
- Found by: xo-4c (`xo test --explore=20 --std-module=ui`: "Hosts on pg_bus hear each other's appends through NOTIFY" deadlocked under seed 2; a likely cause of a stalled linux/arm64 gate)
- Go reference: none
- Test: std/ui "a subscriber that joins pg_bus after it listens still hears -1" (deadlocks before); the NOTIFY test under `--explore=300`
- Timing: none (one atomic read per subscribe)
- Shipped in: not yet released

pg_bus told its subscribers it was listening (version -1, "read the
store") only if they joined before its LISTEN connection was up; one that
joined later never heard it, so code waiting for it waited forever. A
subscriber that joins while the bus listens now hears -1 at once. It may
hear -1 twice when it joins as the LISTEN comes up; -1 only asks for a
store read, so a repeat costs one read.

## 2026-11-16: perf: pages shown a version already rendered take the scope's locks shared

- Kind: perf
- Commit: this push
- Found by: task #162 (std/ui live push frames: 1,000 viewers of one room, the server's sys time)
- Go reference: none
- Test: std/ui "a page shown a version already rendered takes the scope's locks shared (#162)" (a reader holds both locks; the next frame no longer waits for it); "a thin live page is sent each new view over its socket, rendered once per version and view"
- Timing: linux/arm64 host 4 (8 CPUs), one room, 1,000 push viewers, 200 changes every 20 ms, XO_RTSTATS=1, two rounds: server CPU user/sys 3.23/5.27 and 3.12/5.08 s before, 2.80/3.46 and 2.56/3.09 s after; mutex wakes 107k and 113k before, 2.4k and 1.0k after; frames unchanged (200k). linux/arm64 host 1 (20 CPUs), the same run, three rounds each: user/sys 3.47/6.51, 3.46/6.63 and 3.33/6.66 s before, 4.07/4.11, 4.09/4.14 and 3.83/4.01 s after (sys 0.62, total CPU 0.81)
- Shipped in: not yet released

Each new version wakes every page showing the scope at once, and each took the scope's state and frame locks as a writer to read a model and a frame that were already there, one wake after another. A page now reads them under a shared lock and takes the lock as a writer only to catch the scope up or render a frame not yet made.

## 2026-11-16: perf: live pages get the new view over their socket (decision 0689)

- Kind: perf
- Commit: this push
- Found by: owner track (1,000 live viewers on one room, step 4 measurement)
- Go reference: none
- Test: std/ui "a thin live page is sent each new view over its socket, rendered once per version and view" (two pages share one frame, another query gets its own, a page without push=1 still gets versions); every headless Chrome suite
- Timing: the Mac, one room, 200 changes every 20 ms, viewers in processes of 100 on the same machine (~/xo-lead/notes/stdui/viewers): 1,000 viewers 39.4 s of server CPU, 150,130 sync requests and a write p99 of 18 ms before, 11.9 s, no requests and 5.5 ms after; 100 viewers 3.0 s before, 0.9 s after; every viewer current as the writer finished in both
- Shipped in: not yet released

A thin page's live websocket asks for views (`?push=1&q=` its query):
each new version comes as the view itself, rendered and encoded once per
version and view in the scope and written to every page showing it, so
the page shows it without a sync request. A page with Msgs waiting, a
sync in flight, a predicting client or another query still syncs. Pages
of an older runtime, and predicting clients, keep getting versions.

## 2026-11-16: perf: group commit, the syncs to one scope that arrive during an append share the next one (decision 0686)

- Kind: perf
- Commit: this push
- Found by: owner track (hot rooms, step 1 and 2 measurements)
- Go reference: none
- Test: std/ui "syncs that arrive while an append is in flight share the next append" (six syncs, two appends); "many concurrent syncs to one scope are each applied once" (20 clients x 10); the racing-Scopes and lost-race tests; every headless Chrome suite
- Timing: Postgres in Docker, one scope, 52 concurrent clients, 2,080 Msgs (~/xo-lead/notes/stdui/pghot): one instance 9.2 s with 1,890 appends and a p99 of 3.2 s before, 0.23 s with 103 appends and a p99 of 7 ms after; four instances on pg_bus 15 s with 2 syncs failing (100 lost races) before, 0.84 s with 321 appends and none failing after
- Shipped in: not yet released

A sync with Msgs waits in its scope's queue; the request that finds no writer appends everything queued as one batch, then what queued during that append, until the queue is empty, and each waiter gets its own answer. Syncs without Msgs still answer from the instance's copy.

## 2026-11-16: perf: a writer appends at once and reads the store only after a lost race (decision 0686)

- Kind: perf
- Commit: this push
- Found by: owner track (leases for hot scopes; measuring them on real S3)
- Go reference: none
- Test: std/ui "a writer alone on a scope appends at once": 10 changes cost 10 bucket requests (40 before); a stale copy loses once, reads and appends after the other writer; an answer with no append (a refusal, a gap in a client's seqs) still comes from the store's state (the racing-Scopes tests)
- Timing: real S3, ca-central-1, 4 processes x 100 appends to one scope: 64 s before, 24 s after (400 of 400 kept both times); LocalStack and the in-memory bucket agree on the request counts
- Shipped in: not yet released

Scope.commit read the store before every append. A copy the Host keeps current (bus notices, or a writer alone on a scope) now appends at once, and reads the store only after it loses the race; a change that appends nothing reads first, so its answer is never from a stale copy.

## 2026-11-16: feature: object_store, scoped logs in an S3 style bucket (decision 0686)

- Kind: feature
- Commit: this push
- Found by: owner (an S3 store with a conditional write)
- Go reference: none
- Test: std/ui tests on an in-memory bucket: two Hosts act as one, a third starts from the snapshot, a stale append behind a snapshot loses; a snapshot inside a batch keeps that batch for a new instance; a catch-up costs one list and one get per batch. Outside the repo (adapter over github.com/xo-lang/aws service/s3): 4 processes x 150 appends on LocalStack gave 600 of 600 in 14 s; 4 x 100 on real S3 (ca-central-1) gave 400 of 400 in 64 s, and S3 refused a second If-None-Match put
- Timing: none
- Shipped in: not yet released

`ui.Objects` (create only if new, put, get, list) and `ui.object_store(objects, prefix:)`.

## 2026-11-16: feature: scopes in a Host, a room or a user each with its own log (decision 0686 step 3)

- Kind: feature
- Commit: this push
- Found by: owner (several scopes per page; decision 0686 plan step 3)
- Go reference: none
- Test: std/ui tests: rooms by path each with their own log, versions and live pages, a path no scope gives is 404; a scope per principal where a sync naming any page reaches only its sender's scope; at most max_scopes held, the least recently used without a live page dropped and read back from the store; internal/uiruntime TestScopedSyncURLKeepsPage (the runtime added "?thin=1" to a sync URL that had a query; fails before); every headless Chrome suite
- Timing: none
- Shipped in: not yet released

`host(..., scope_of:, max_scopes:)` and the same on `host_with`; `apply`, `try_apply` and `model` take `scope:`. `Bus.subscribe` (not yet in a release) now takes one `fn(Str, Int)` for every scope; it took a scope and a `fn(Int)`.

## 2026-11-16: feature: pg_bus, notices between instances by Postgres LISTEN/NOTIFY (decision 0686)

- Kind: feature
- Commit: this push
- Found by: decision 0686 plan
- Go reference: none
- Test: std/ui tests: Hosts on pg_bus over an in memory LISTEN/NOTIFY driver hear each other's appends and wake their live connections; a resync notice (-1) makes a Host read the store it shares; after a resync a live page is sent the version it must sync to (it got -1 before the fix, which the runtime ignores); on Postgres 17 a watching process heard another process's append and held all 3 items
- Timing: none
- Shipped in: not yet released

`ui.pg_bus(db, tasks, clock, channel:)`. A Bus subscriber now hears -1 when notices may have been missed (a reconnect), and a Host then reads the store.

## 2026-11-16: feature: sql_store, scope logs in Postgres (decision 0686)

- Kind: feature
- Commit: this push
- Found by: decision 0686 plan
- Go reference: none
- Test: std/ui tests: two Hosts on one SQL store (std's in memory driver) converge and a third starts from the snapshot, a stale append loses, scopes share the tables apart; a driver whose CREATE fails as Postgres's does; a store losing 40 races in a row; four processes appending 300 Msgs each to one scope on Postgres 17 gave 1200 items, none lost or doubled
- Timing: four processes, 1200 appends to one scope on Postgres 17 in Docker on the Mac: 3 s (one run)
- Shipped in: not yet released

`ui.sql_store(db, table:)`. Two fixes found by the Postgres run, each with a test that failed before: instances starting together no longer fail on Postgres's CREATE TABLE IF NOT EXISTS race (gotchas/postgres-create-if-not-exists-race.md), and a change takes up to 100 lost races, not 16 (four writers at full speed lost 16 in a row).

## 2026-11-16: feature: dir_store, a scope log in a directory (decision 0686)

- Kind: feature
- Commit: this push
- Found by: decision 0686 plan
- Go reference: none
- Test: std/ui tests: two Hosts on one directory store converge and a third starts from its snapshot, a stale append loses, a snapshot inside a batch keeps that batch; three processes appending 300 Msgs each to one directory on disk at once give 900 items, none lost or doubled
- Timing: none
- Shipped in: not yet released

`ui.dir_store(fs, dir)`: one directory per scope, a file per batch, a snapshot file. Appends hold the scope's file lock and rename the batch into place.

## 2026-11-16: feature: scoped logs in a store, Hosts that act as one behind a load balancer (decision 0686 stage 1)

- Kind: feature
- Commit: this push
- Found by: owner (several instances behind a load balancer)
- Go reference: none
- Test: std/ui tests: Scopes on one store converge with each sync routed to a random one of 2 to 4, with loss, reordering, offline clients, refusals and compaction (30 runs; with a store per node, as separate instances today, it fails); two Scopes racing for one version; two Hosts on one store and bus (live notice, page, retry not applied twice, principal kept, server Msgs); a Host appends after another without a bus; compaction snapshots and a new Host starts from the snapshot; the ui corpus; every headless Chrome suite (todo 21, morph 3, navigation 17, redeploy 6, pages 14, busy 6, shop 12, shop tab 12)
- Timing: examples/ui-shop/measure, one cart add with 2, 20, 200 users: 23, 9, 12 us before and 16, 10, 14 us with the memory store (one sample each)
- Shipped in: not yet released

`ui.host_with(app, store, bus, scope:)`, `Store` (a conditional `append`, `read`, `snapshot`, `put_snapshot`), `memory_store()`, `Bus` and `local_bus()`, `Scope` (a Server kept in step with a store), and `Host.try_apply`. Store, Bus and Scope take an effect parameter, so a Host performs what its store performs. `ui.host` keeps working as one instance. `Host.apply` and `Host.model` now declare `uses sync`.

## 2026-11-16: perf: with redact, a change no longer computes every viewer's digest on its request

- Kind: perf
- Commit: this push
- Found by: decision 0672's measurement (the digest work ran under the live connections' lock, on the request of the sync that made the change)
- Go reference: none
- Test: std/ui tests (a live connection hears only of changes to its own view; versions fold into one while a connection is busy); headless Chrome shop 12/12, shop tab 12/12 and every earlier suite
- Timing: Host.apply with 200 and 2,000 live principals (std/ui's todo app, the test's redact): the request's time per change 607 us to 6 us and 74,766 us to 56 us; the connections' digests (now in their own tasks, in parallel) 636 us and 73,111 us in all
- Shipped in: not yet released

A change now only sends the new version to each live connection's channel, as without redact; each connection's task asks whether its principal's view changed (`news`) and writes only then.

## 2026-11-16: perf: a filter on a page with the App's client renders without a fetch

- Kind: perf
- Commit: this push
- Found by: decision 0672's prototype (a filter took one round trip even with the wasm client)
- Go reference: none
- Test: headless Chrome drive-shop-tab.mjs 12/12 with the wasm client and with a thin one; navigation 17/17 (back from an App page to a server page, which a first version rendered as the App), todo 21, morph 3, redeploy 6, pages 14, busy 6, shop 12
- Timing: a filter link at 150 ms of added latency: 204 to 0 ms with the wasm client (thin: 214 ms, unchanged)
- Shipped in: not yet released

A link or GET form to the running App's own path with another query (tab state, `with_url_view`) is rendered by the App's client with the new query and the URL changes, without fetching the page; a thin client, another path or a frame still fetches.

## 2026-11-16: feature: tab state in the URL (App.with_url_view)

- Kind: feature
- Commit: this push
- Found by: decision 0672's prototype (examples/ui-shop: filters and sort kept in the URL)
- Go reference: none
- Test: std/ui tests (the page, a sync and a prediction render the page's query); internal/uiruntime TestSyncSendsPageQuery; headless Chrome ~/xo-lead/notes/stdui/e2e/drive-shop-tab.mjs 12/12 (filter and sort without a full load, back, reload, a shared link, another user's page untouched); every earlier suite (todo 21, morph 3, navigation 17, redeploy 6, pages 14, busy 6, shop 12)
- Shipped in: not yet released

`App.with_url_view(fn(model, query) -> Html)`: the view also reads the page's query, so tab state (a filter, a sort, a selected row) lives in the URL and survives reload, back and forward and a shared link, rendered by the server on a first load. The runtime sends the page's query with each sync (`Up.query`) and each prediction (`ClientIn.query`).

## 2026-11-16: perf: with redact, another user's change no longer wakes every viewer

- Kind: perf
- Commit: this push
- Found by: decision 0672's baseline measurement (examples/ui-shop: carts in the shared model with redact and allow)
- Go reference: none
- Test: std/ui tests (a sync answers same while the principal's view did not change; a live connection hears only of changes to its own view); internal/uiruntime TestSyncSendsSeenDigest
- Timing: examples/ui-shop/measure, one user's cart add with 2, 20 and 200 users: viewers whose sync was not `same` 1/1, 19/19, 199/199 to 0 in each; bytes per other viewer's sync 1180 to 1192 to 154 to 156 (thin) and 1495 to 1508 to 154 to 156 (wasm)
- Shipped in: not yet released

With `redact`, every version change woke every live page, and each page's sync rendered and sent the whole view though that viewer's part had not changed. The page and each sync now carry a digest of what the principal is shown (`data-xo-seen`, `Sync.seen`), the runtime sends it back (`Up.seen`), and the host answers `same` while it matches; a live connection is told of a new version only when its principal's digest changed.

## 2026-11-16: perf: render checks names over bytes and writes each piece

- Kind: perf
- Commit: this push
- Found by: task #88 (performance Early to Proven), the module's first Go twin (bench/perf/ui against x/net/html v0.60.0's Render: render 1.171 on the Mac)
- Go reference: golang.org/x/net/html Render
- Test: the std/ui tests (tag and attribute names refused); bench/perf/ui twincheck output matches Go
- Timing: bench/perf/ui on the Mac, Xo over Go: render 1.171 to 0.892 (twincheck -time 5)
- Shipped in: not yet released

render made a list of runes for every tag and attribute name it checked, and built each tag and attribute through string interpolation. A name's allowed characters are ASCII, so the check walks its bytes; the pieces go into the builder one by one.

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
