# 0689: std/ui live pages get the new view over their socket

Status: Accepted (owner, 2026-11-16)

## Context

A live page learned of a new version over `GET /_xo/live` (the version as
text) and then made a sync request (`POST /_xo/sync`, no Msgs) to fetch
the view. Each of those requests reads the scope's copy, renders the view
for that page and encodes a Sync, so a change in a room of V viewers cost
V HTTP requests and V renders of the same view.

Measured on the Mac (one room, 200 changes every 20 ms, viewers in
processes of 100 on the same machine, ~/xo-lead/notes/stdui/viewers):
1,000 viewers made 150,130 sync requests, the server spent 39.4 s of CPU
(9.6 s user, 29.8 s system: HTTP per request, not rendering), and a
write's p99 was 18 ms. Per viewer sync about 0.2 ms of server CPU; per
change about 0.2 s across 1,000 viewers.

## Decision

- A live connection that asks (`?push=1&q=QUERY`, QUERY the page's query
  without "?") is sent each later version as its view: JSON
  `{"ver", "html", "seen", "build", "q"}`, the same fields a Sync without
  Msgs answers with. The first message is still the version as text.
- The frame is made by `Host.frame`: the scope's copy read with its version
  under the scope's lock, the view rendered for the page's query and
  principal (`shown`, with redact), encoded, and kept in the scope's
  `frames` map by view (query and seen digest) and version. Every page
  showing that view at that version is written the same text: one render
  and one encode per version and view. The map holds at most 256 views and
  is cleared when full.
- The runtime asks for frames when it is a thin client (no wasm module).
  On a frame it shows the view at once when it is newer and nothing
  waits: no Msgs queued, no sync in flight, not predicting, the same
  query. Otherwise it syncs as before. A version as text still makes it
  sync.
- A connection that does not ask (an older runtime's page, a predicting
  client, any other client) gets versions as before.

## Consequences

- Same run with frames: 1,000 viewers made no sync requests, the server
  spent 11.9 s of CPU (3.1 user, 8.8 system), and a write's p99 was 5.5 ms;
  100 viewers 0.9 s against 3.0 s. Every viewer was current as the writer
  finished, as before.
- What is left is one websocket write per viewer per change (about 15 us
  user and 44 us system each on the Mac). A frame is sent for every
  version the connection's task sees; versions that come while a write is
  in flight fold into one (the one slot channel), as notices did.
- With redact the digest is still computed per connection per version
  (`news`), and views are cached per seen digest, so principals shown the
  same thing share a frame.
- Rendering happens under the scope's frames lock, so one scope's renders
  for its live pages run one at a time; other scopes are not held.
- The frame carries `q`; a page whose query changed since it connected
  syncs instead of showing a view for the old query.

## Alternatives

- Keep version notices and make sync cheaper: the HTTP request is most of
  the cost (system CPU), so it stays V requests per change.
- Send the log entries and let pages fold them: thin pages cannot run
  update; predicting clients could, and may later (they still sync).
- Send a diff of the HTML: smaller frames, but a diff per view per
  version, and the runtime already morphs the whole view in place.
- Render per connection without a cache: V renders per change of the same
  view, which the cache removes.
