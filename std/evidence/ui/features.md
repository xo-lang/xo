# std/ui feature audit (decision 0691)

Every feature std/ui's decisions (0659, 0663, 0664, 0668, 0670, 0672,
0686, 0689) and its spec/std.md section promise, each with the test that
shows it (`std/ui_test.xo:NAME` is the `test "NAME"` there). A feature
that is promised and not built, or built with no automated test, is an
open item. Keep this list current in each std/ui change (0691 item 4).
First written 2026-11-16 (task #186, xo-4c) against origin/main e859cd7f0.

## Views and events (0659 stage 1)

- [x] Html views escape text and attribute values | 0659 | `std/ui_test.xo:render escapes text and attribute values`
- [x] void elements render without a closing tag | 0659 | `std/ui_test.xo:void elements have no closing tag`
- [x] unsafe URLs in attributes are replaced | 0659 | `std/ui_test.xo:unsafe URLs are replaced`
- [x] on() carries the Msg as JSON in a data attribute | 0659 | `std/ui_test.xo:on renders the Msg as JSON in a data attribute`
- [x] field() puts a form value in the event's JSON | 0659 | `std/ui_test.xo:field marks a form value in the event's JSON`
- [x] the page carries the rendered view and the state | 0659 | `std/ui_test.xo:the page carries the rendered view and the state`

## Sync and reconciliation (0659 stages 1 and 2)

- [x] a replica predicts, then the server confirms | 0659 | `std/ui_test.xo:a replica predicts, then confirms`
- [x] a duplicate or out of order Msg is applied once, in order | 0659 | `std/ui_test.xo:a duplicate or out of order Msg is applied once, in order`
- [x] the server overturns a prediction it rejects | 0659 | `std/ui_test.xo:the server overturns a prediction it rejects`
- [x] confirmed Msgs wait for the server | 0659 | `std/ui_test.xo:a confirmed Msg waits for the server instead of showing a guess`
- [x] a refused confirmed Msg never showed as done | 0659 | `std/ui_test.xo:a confirmed Msg the server refuses never showed as done`
- [x] replicas converge over loss, reordering, refusal and compaction, for any seed | 0659 | `std/ui_test.xo:convergence holds for any seed`
- [x] log compaction: a client behind it restarts from a snapshot | 0659 | `std/ui_test.xo:a client behind a compacted log restarts from a snapshot`
- [x] the host keeps at most twice keep log entries | 0659 | `std/ui_test.xo:a host keeps at most twice keep log entries`
- [x] server Msgs (Host.apply) reach every replica in order | 0659 | `std/ui_test.xo:a server Msg reaches every replica in order`
- [x] the server's client id is reserved | 0659 | `std/ui_test.xo:Host.apply changes the page, and a sync may not use the server's id`

## Transport (0659)

- [x] the host serves the page, the runtime and sync | 0659 | `std/ui_test.xo:the host serves the page, the runtime and sync`
- [x] a thin sync leaves out the log entries | 0659 | `std/ui_test.xo:a thin sync leaves out the log entries`
- [x] sync refuses other content types and bad bodies | 0659 | `std/ui_test.xo:sync refuses other content types and bad bodies`
- [x] the host mounts under a prefix | 0659 | `std/ui_test.xo:the host mounts under a prefix`
- [x] an idle sync at the current version answers same | 0659 | `std/ui_test.xo:an idle sync at the current version answers same, without the page`
- [x] a change tells the live connections | 0659 | `std/ui_test.xo:a sync that changes the model tells the live connections`
- [x] the page names its live endpoint | 0659 | `std/ui_test.xo:the page names the live endpoint`
- [x] the runtime and client at stamped URLs, cached for good | 0659 | `std/ui_test.xo:the runtime and client are served at stamped URLs, cached for good`
- [x] the client module is served gzipped with an ETag | 0659 | `std/ui_test.xo:the client module is served gzipped when accepted, with an ETag`

## Browser runtime (0659)

- [x] two tabs of one browser are two clients | 0659 | `internal/uiruntime/runtime_test.go:TestTwoTabsAreTwoClients`
- [x] the queue persists across reloads and a closed tab's queue is adopted | 0659 | `internal/uiruntime/runtime_test.go:TestClosedTabQueueIsAdopted`
- [x] a signed out page keeps its queue; a 403 drops it | 0664 | `internal/uiruntime/runtime_test.go:TestForbiddenDropsQueue`
- [x] the view is patched in place, keeping nodes and focus | 0659 | `internal/uiruntime/runtime_test.go:TestMorphKeepsNodes`
- [x] a new build reloads once the queue drains | 0659 | `internal/uiruntime/runtime_test.go:TestNewBuildReloadsAfterQueue`
- [x] opening the socket syncs | 0659 | `internal/uiruntime/runtime_test.go:TestSocketOpenSyncs`
- [x] browser measurement (bundle size, time to resume) | 0659 | `std/evidence/ui/browser.md` (headless Chrome, ui-todo: 104 KB cold, resume 7 ms unthrottled and 866 ms on slow 4G; no size guard yet)

## Client in wasm (0659 stage 3, 0663)

- [x] the wasm client predicts with the App's own update | 0659 | `std/ui_test.xo:predict folds pending Msgs over the confirmed model and renders it`
- [x] the client leaves confirmed Msgs out of a prediction | 0659 | `std/ui_test.xo:predict leaves confirmed Msgs out and counts them`
- [x] a host with a client serves its wasm and names it | 0659 | `std/ui_test.xo:a host with a client serves its wasm and names it in the page`
- [x] a host without a client is a thin one | 0659 | `std/ui_test.xo:a host without a client is a thin one`
- [x] a page with a client preloads its module | 0659 | `std/ui_test.xo:a page with a client preloads its module in the head`
- [x] embed.build builds the client into the server | 0663 | `cmd/xo/embedbuild_test.go:TestEmbedBuild`
- [x] embed.build refuses nesting and self builds | 0663 | `cmd/xo/embedbuild_test.go:TestEmbedBuildErrors`

## Authorization and redaction (0664 stage 4a)

- [x] a client is bound to its principal | 0664 | `std/ui_test.xo:receive_as binds a client to its principal and refuses others`
- [x] allow refuses a Msg: acknowledged, counted, not applied | 0664 | `std/ui_test.xo:a refused Msg is acknowledged, counted and not applied`
- [x] auth answers 401 and 403 | 0664 | `std/ui_test.xo:a host with auth answers 401 without a principal and 403 for another's client`
- [x] signin_url sends a page request without a principal there | 0664 | `std/ui_test.xo:signin_url sends a page request without a principal there`
- [x] redact shapes the page and every sync, with no log entries | 0664 | `std/ui_test.xo:redact shapes the page and every sync, and no log entries are sent`
- [x] with redact, a sync answers same while the view is unchanged | 0664 | `std/ui_test.xo:with redact, a sync answers same while the principal's view did not change`
- [x] with redact, a live connection hears only of its own view | 0664 | `std/ui_test.xo:with redact, a live connection hears only of changes to its own view`
- [ ] commands: update returns server commands whose results come back as Msgs | 0664 stage 4b | not built
- [ ] `server` fields never sent to a client | 0659 point 5, 0664 stage 4b | not built (redact blanks fields for now)
- [ ] query subscriptions (partial replication) | 0659 stage 4, 0664 stage 4c | not built

## Pages (0668, 0670)

- [x] plain renders a server page the runtime takes over | 0670 | `std/ui_test.xo:plain renders a server page the runtime takes over`
- [x] serve_runtime serves the runtime without an App | 0670 | `std/ui_test.xo:serve_runtime serves the runtime without an App`
- [x] links between pages without a full load, back and forward restore scroll | 0668 | `internal/uiruntime/nav_test.go:TestLinkNavigatesInPlace`, `TestBackRestoresScroll`
- [x] GET forms in place, submit on input | 0670 | `internal/uiruntime/nav_test.go:TestGetFormInPlace`
- [x] POST forms: 303 followed, 422 shown in place, multipart | 0670 | `internal/uiruntime/nav_test.go:TestPostForm`
- [x] frames patch one element | 0670 | `internal/uiruntime/nav_test.go:TestFramePatchesOneElement`
- [x] opting out: data-xo-reload, target, download, modifier keys | 0668, 0670 | `internal/uiruntime/nav_test.go:TestOptOutsLoadNormally`

## Viewer state (0672)

- [x] tab state: with_url_view renders the page's query | 0672 | `std/ui_test.xo:with_url_view renders the page's query on the page, a sync and a prediction`
- [x] a sync sends the page's query | 0672 | `internal/uiruntime/runtime_test.go:TestSyncSendsPageQuery`
- [ ] user state across devices (a typed per-user part, views over two scopes) | 0672, 0686 plan step 6 | not built
- [ ] 0672 plan steps 2 to 4: 2/20/200 user measurements, two-device property test | 0672 | open

## Scopes, stores and buses (0686)

- [x] scope_of gives each page its own log, versions and live pages | 0686 | `std/ui_test.xo:scope_of puts each page in its own scope: own log, own versions, own live pages`
- [x] a scope per principal | 0686 | `std/ui_test.xo:a scope per principal: a sync naming any page reaches only its sender's scope`
- [x] max_scopes drops the least recently used scope | 0686 | `std/ui_test.xo:a Host keeps at most max_scopes scopes, dropping the least recently used without a live page`
- [x] Scopes on one store converge with syncs routed anywhere | 0686 | `std/ui_test.xo:Scopes on one store converge with syncs routed to any of them (decision 0686)`
- [x] two Hosts on one store and bus act as one | 0686 | `std/ui_test.xo:two Hosts on one store and bus act as one, behind a load balancer (decision 0686)`
- [x] a Host catches up before it appends | 0686 | `std/ui_test.xo:a Host on a store it shares catches up before it appends`
- [x] a lost append race applies again after the winner | 0686 | `std/ui_test.xo:two Scopes racing for one version: the loser applies again after the winner`
- [x] a change outlasts a run of lost races | 0686 | `std/ui_test.xo:a change outlasts a run of lost races (four writers on one Postgres scope lost 16 in a row)`
- [x] store compaction and a new Host starting from the snapshot | 0686 | `std/ui_test.xo:compaction snapshots the scope, the store drops what it covers, and a new Host starts from it`
- [x] memory store | 0686 | `std/ui_test.xo:a memory Host declares only sync`
- [x] directory store | 0686 | `std/ui_test.xo:two Hosts on one directory store act as one, and a third starts from its snapshot`
- [x] object (S3) store | 0686 | `std/ui_test.xo:two Hosts on one bucket act as one, a third starts from its snapshot, and a stale append loses`
- [x] SQL store | 0686 | `std/ui_test.xo:two Hosts on one SQL store act as one, and a third starts from its snapshot`
- [x] Postgres LISTEN/NOTIFY bus | 0686 | `std/ui_test.xo:Hosts on pg_bus hear each other's appends through NOTIFY`
- [x] a bus reconnect makes a Host read the store | 0686 | `std/ui_test.xo:a resync notice makes a Host read the store it shares (a bus reconnect)`
- [x] group commit: concurrent syncs share an append | 0686 | `std/ui_test.xo:syncs that arrive while an append is in flight share the next append (group commit)`
- [x] leases: a hot scope's syncs forwarded to one instance | 0686 item 6 | `std/ui_test.xo:an instance that loses a race takes a lease, and the others forward that scope's syncs to it`
- [ ] polling bus for the object store | 0686 plan step 5 | not built
- [ ] 2/20/200/2,000 user measurements over several instances | 0686 plan step 4 | partial (four writers on Postgres, localstack, one real S3 run)
- [ ] real-time rooms and the canvas game measurement | 0686 plan step 7 | open

## Live push views (0689)

- [x] a live page is sent each new view over its socket, rendered once per version and view | 0689 | `std/ui_test.xo:a thin live page is sent each new view over its socket, rendered once per version and view (decision 0689)`
- [x] after a resync a live page is sent a real version | 0689 | `std/ui_test.xo:after a resync a live page is sent the version it must sync to, never -1`
- [x] pages shown a rendered version share the scope's locks | 0689 | `std/ui_test.xo:a page shown a version already rendered takes the scope's locks shared (#162)`
