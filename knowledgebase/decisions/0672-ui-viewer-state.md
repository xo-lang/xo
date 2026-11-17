# 0672: std/ui per-viewer state (layer 3)

Status: Proposed (owner session, 2026-11-16). Draft: the design below is
to be prototyped (examples/ui-shop) and measured before the owner
decides.

## Context

std/ui has three layers (decision 0670's table): pages and navigation
(0668, 0670), shared live data (the App model of 0659), and per-viewer
state, which this decision adds.

An App's model is shared by every viewer of its Host. State that belongs
to one viewer has no place today:

- tab state: a filter, a sort order, a selected row, a wizard step, an
  open panel; it belongs in the URL so back, forward and a shared link
  work, and the server must render it on a first load;
- user state: a cart, a draft, saved preferences; it follows the user
  across tabs and devices and survives going offline.

Putting either in the shared model leaks it to every viewer, and putting
it in the browser alone loses server rendering, other devices and the
server's check of what it emits. Other frameworks:

- LiveView keeps a server process per connection holding that
  connection's state, with `handle_params` mapping the URL in. Every
  event is a round trip; nothing works offline.
- Elm's `Browser.application` keeps the URL in the one client model.
  No server rendering, no state across devices.
- Replicache, Zero and Convex scope data by user on the server and sync
  each user's slice to their clients; local state stays in the client
  framework.

What works today, and the baseline the prototype is measured against:
keep every user's state in the shared model (`carts: Map[Str, Cart]`),
`redact` each viewer's copy down to their own entry, and `allow` a Msg
only for the principal it names. It leaks nothing, but every user's
changes go through one log, wake every live client, and hold every
user's state in one model; tab state and URLs are not covered at all.

## Decision (draft)

An App gains two optional parts beside its shared model, each with its
own model, Msg type, update and place in the view. Same rules as the
shared update: pure (decision 0662), run in the wasm client to show at
once and on the server, which decides.

1. Tab state, `T` with Msgs `TA`:
   - `tab_init(query: url.Values) -> T` builds it from the page URL: on
     the server for a first load and a shared link, and in the runtime
     on back and forward (no fetch: the client re-inits from the URL).
   - `tab_update(m: M, t: T, a: TA) -> (T, List[E])` may read the
     shared model and may emit shared Msgs (a filter does not; a
     "reserve these seats" step does), which then go through `allow`
     like any Msg.
   - `tab_url(t: T) -> Str` is the query string T is kept in; the
     runtime replaces the history entry when it changes, and pushes one
     for an act `tab_push(a)` says is a step (like `confirm` for Msgs).
   - The server stores nothing: a thin client sends its T with the act
     and gets T and the page back; the wasm client computes T itself.
     T from a client is input like any request body: decoded, never
     trusted beyond that viewer's own page.
2. User state, `U` with Msgs `UA`:
   - one log per principal (the Server and Replica machinery of 0659,
     keyed by principal), synced to every tab and device of that user,
     with the same offline queue, acks and refusals;
   - `user_update(m: M, u: U, a: UA) -> (U, List[E])`: a checkout
     empties the cart and emits the shared `PlaceOrder`;
   - `user_init: U` for a principal with no log yet; the Host's `keep`
     bounds each log; a store hook (memory by default) persists it, and
     the store is its own stage;
   - without `auth` there is one principal (""), so user state would be
     shared by every viewer: the Host refuses an App with user state and
     no `auth`.
3. The view takes all three: `view(m: M, u: U, t: T) -> Html`, with `m`
   redacted for the viewer first (0664). Events name their target:
   `ui.on(ev, msg)` a shared Msg as today, `ui.on_user(ev, ua)`,
   `ui.on_tab(ev, ta)`.
4. Sync carries up to three streams (shared, user, tab) in one request;
   an App without the new parts sends what it sends today.
5. Data loaded per viewer from a database (search over an index, a page
   of rows) is not state: it stays a server page (0670) or becomes a
   query subscription (0659 stage 4), its own decision.

## Baseline measured (2026-11-16)

examples/ui-shop holds the baseline (carts in the shared model, `redact`
to the viewer's cart, `allow` binding a Msg's user to the principal) and
`measure/`, which drives the Host in process: N users each add a line,
then one user adds again and every other user syncs.

| users | model JSON | others not `same` | bytes per other sync (thin, wasm) |
|---|---|---|---|
| 2 | 334 | 1 of 1 | 1180, 1495 |
| 20 | 614 | 19 of 19 | 1184, 1500 |
| 200 | 3594 | 199 of 199 | 1192, 1508 |

Redact kept each sync's bytes flat, but every change woke every viewer
for a full page of an unchanged view: traffic grows with the square of
the active users. Most of that is fixed in the Host without new API: the
page and each sync carry a digest of what the principal is shown
(`Sync.seen`, sent back as `Up.seen`), a sync whose digest matches
answers `same`, and a live connection is told only when its principal's
digest changed. With it: 0 of 1, 0 of 19, 0 of 199 woken; 154 to 156
bytes per sync. That change ships on its own (std/changes/ui.md
2026-11-16).

Chrome (two users, two browsers, ~/xo-lead/notes/stdui/e2e/drive-shop.mjs):
12/12 with the digest; built without it, the other user's page synced
on each cart click (2 to 3 syncs). The example also showed that in the
baseline the view learns who is viewing only through `redact` (the
shop's redact keeps an empty cart for the viewer so the view can name
them): the three-argument view of the decision makes that explicit.

The digests' cost on the server (2026-11-16, Mac): with the work on the
request under the connections' lock, Host.apply took 607 us a change at
200 live principals and 74.8 ms at 2,000 (std/ui's test App, whose
redact scans every todo: quadratic). Moved into each connection's task
(each asks whether its principal's view changed), the request takes 6
and 56 us and the digests run in parallel, 636 us and 73 ms in all. The
total is linear when redact is: the shop's (one cart) costs 0.6 us a
user, 9.8 ms a change at 20,000 users.

What remains for user state after it: every cart click still goes
through the one shared log and lock, the model holds every user's cart
(about 15 bytes per cart line here), the server computes one digest per
connected principal per change, and tab state is not covered at all.

## Tab state prototyped (2026-11-16)

Built smaller than the draft's part 1, to see how far it goes: tab state
is the page's query and nothing else. `App.with_url_view(fn(M,
url.Values) -> Html)`; no `T`, no `TA`, no `tab_update` or `tab_url`. A
link or a GET form changes it through the page layer (0670: fetch, morph,
history), the runtime sends the page's query with each sync and each
prediction, and the server renders it on a first load.

examples/ui-shop filters the catalog by tag and sorts it by price this
way. Chrome (~/xo-lead/notes/stdui/e2e/drive-shop-tab.mjs) 12/12: filter
and sort without a full load, the other filter kept by the links, back,
reload (server rendered), a cart add while filtered, a shared link
opened by another user (their own cart, the same filter), and the other
user's page untouched (no sync). A filter showed with the URL change on
localhost and in 204 ms at 150 ms of added latency: one round trip,
since even the wasm client fetches the new URL.

What it does not cover, against the draft:

- state that should not be in the URL or history (an open menu, a
  half-typed field): the morph keeps the DOM's own, which covers most;
- a tab act that computes from the model (a wizard step that checks
  stock): the view can read the model, but the next step is a link or
  form, not an update;
- (done the same day) a filter without a round trip: the runtime's
  visit renders a GET of the App's own path with another query through
  the client (predict takes the query) and changes the URL, without a
  fetch. At 150 ms of latency a filter showed in 0 ms with the wasm
  client, 214 ms with a thin one (which still fetches); back and forward
  between queries render locally too. Every earlier suite passed after a
  fix the navigation suite found: on back, `location` is already the
  entry's URL, so the check compares with the path the App booted on.

So far the query alone holds the shop's tab state, and the typed `T`
and `TA` of the draft are not needed for it.

## User state built (2026-11-17)

`App.with_user(init, update, view)` and `on_user(event, act)`, part 2
of the draft with these choices:

- The state is kept as JSON text: `UserPart` erases `U` and `UA` so the
  Host keeps its two type parameters. Each principal's state is a
  `Slot[Str, Str]` in its own scope, `"~user/" + principal`, in the
  Host's store, so it has its own log, `keep`, store and bus key; a
  change publishes that scope and wakes only the principal's live
  connections (`Sub.uwake`): a `"u<ver>"` text message, or with push
  (0689) a frame carrying `uver`.
- User acts go in the sync's `user` part with their own seqs per client
  and the same ack rules as Msgs; a client of another principal is 403.
  An act that does not decode is acknowledged and logged as refused.
- Emitted Msgs are applied after the user commit, each through `allow`
  as the principal's: at most once. If the process dies or the shared
  apply fails between the two writes, the user state moved and the
  Msgs are lost. Exactly once would need the shared entry to carry the
  user act's id (an outbox) and is left for when an app needs it.
- The runtime does not predict user acts (the wasm client would need
  the user update compiled in); an act shows when its sync returns.
- Tab state stays the page's query (above); `view(m, u, query)`.

Test: two devices of ann share the state, bob's is apart, bob on ann's
client is 403, a live page of ann is woken and bob's is not; ann's act
emits a shared Add, eve's is refused by allow. The Node harness checks
the runtime sends user acts apart, once each, and syncs on `u<n>` only
when its state is older.

## Open questions for the prototype

- Are two parts (tab and user) worth the type parameters, or is one
  viewer part with a scope enough for real apps? The shop example needs
  both (filters in the URL, a cart across devices).
- With the digest, is per-user state still worth its API for user data,
  or only for tab state? Measure next: server time per change with 200
  live principals (digests), and the shared log's growth from cart
  clicks against `keep`.
- A user act that emits a shared Msg while offline: the server applies
  the act and its Msgs at sync time against the model it has then; the
  client's prediction may be overturned, as for Msgs.
- How the runtime keeps tab state per tab (sessionStorage beside the
  queue), and how 0670's navigation treats a same-path query change on
  an App with tab state (re-init locally instead of fetching).
- Wasm size: the client carries three updates and the view.

## Plan

1. Prototype `examples/ui-shop`: a shared catalog with stock (shared
   Msgs, a confirmed checkout), filters and sort in the URL (tab
   state), a cart per user across two browsers (user state).
2. Measure against the baseline above (state in the shared model with
   redact and allow): live notices and bytes per sync with 2, 20 and 200
   users, server memory, wasm size, time for a filter to show.
3. Property test: two devices of one user and one of another, random
   acts with drops and reorders, converge; no user's state reaches
   another's view. Done 2026-11-17: std/ui "a principal's devices
   converge on one state and no one else sees it, over a lossy network"
   (ten seeds of 150 ticks) and its seed property: requests and replies
   dropped, replies late, old requests replayed, then a heal; ann's two
   devices end on one state, every post reaches the shared model once,
   no reply to one principal shows the other's draft. Turning the
   server's exactly-once check into at-least-once fails it. It found
   that the user update's `m` is the model the page showed, the same
   for every act of one sync (gotchas/ui-user-step-sees-shown-model.md).
4. Chrome checks: filters survive reload, back and a shared link; a cart
   added offline on one device shows on the other after reconnect; a
   second user never sees it.

## Alternatives

- State in the shared model with redact and allow (the baseline): works
  today, no API change; costs grow with users and tab state is not
  covered.
- A server process per connection (LiveView): no offline, a round trip
  per event, state lost when the connection drops.
- Client-only local state (Elm): no server rendering from the URL, no
  state across devices, no server check of what it emits.
- One Host per user: per-user logs come free, but shared data and the
  user's state cannot meet in one view or one update.
