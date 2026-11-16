# 0686: std/ui scoped logs in a durable store

Status: Proposed (owner session, 2026-11-16).

## Context

A std/ui Host (decision 0659) keeps its App's log and model in memory, in
one process. Behind a load balancer with several instances each instance
has its own model (two users on two instances see different data and
can both buy the last item), a client's de-duplication (its last applied
seq) and principal binding live in one instance's memory (a retry routed
elsewhere is applied again), live notices reach only the connections of
the instance that made the change, and version numbers name different
histories. Sticky sessions do not fix shared data. 0659 listed "more than
one server process" as not covered.

Other systems (owner session, 2026-11-16):

- Turbo, htmx, Livewire, and LiveView after a reconnect keep no state in
  the server process: the database is the truth and a pub/sub (Redis,
  Postgres, Action Cable) fans out to every instance. No offline queue,
  no prediction against shared state.
- Durable Objects, Liveblocks, PartyKit and Figma run one writer per room
  and route every connection of a room to it: scale by rooms, failover
  and routing to run.
- Replicache and Zero keep a versioned log in a database behind stateless
  push and pull endpoints: a push applies a client's mutations with its
  last mutation id in one transaction, a pull returns changes since the
  client's version, and a "poke" tells clients to pull. This is std/ui's
  protocol already: Up is push, Down is pull, acked is the last mutation
  id, a live notice is the poke.

The owner asked for the most scalable and flexible design, not the
smallest change, with few options, working with more than one kind of
storage (S3 as well as Postgres), and fit for games.

## Decision

State lives in scoped logs in a durable store; any instance serves any
client.

1. Scopes. An App's state is split into scopes, each with its own log
   and model: the whole App, a room, a tenant, a user. A page belongs to
   one scope (a Host function maps a request to its scope key; "" by
   default, one scope). Throughput grows with scopes as well as
   instances: there is no lock for the whole App.
2. Store. One interface, whose only atomic operation is a conditional
   create, so every durable backend can implement it:
   - append(scope, after, batch): writes a batch of log entries as the
     versions after `after`, only if no entry `after + 1` exists yet;
     otherwise it reports the version taken;
   - read(scope, after): the batches after a version, in order;
   - snapshot and put_snapshot(scope, ver, state): the model, each
     client's last seq and principal at a version, for a cold instance
     and for compaction.
   Each entry carries its client, seq and principal (stored, never sent
   to clients), so replay rebuilds de-duplication and principal binding:
   nothing about a client lives outside the log. Backends: memory (tests,
   one process), a local directory (exclusive create), Postgres or SQLite
   (`UNIQUE (scope, ver)`), S3 (`If-None-Match: *`, a batch per object,
   the tail read by GET of the next key until 404; GCS, Azure Blob and
   DynamoDB have the same conditional create).
3. Accepting Msgs. An instance keeps a cached Server per active scope.
   A sync carrying Msgs brings the cache up to the store's version (reads
   the batches it lacks), runs allow and update, and appends the new
   entries after the version it read. If another instance appended first,
   the append fails: the instance reads the winner's batch and runs the
   Msgs again (optimistic concurrency, as in event sourcing). A sync
   without Msgs answers from the cache when the bus says nothing newer
   was appended; it never touches the store.
4. Bus. Notices are a second interface, chosen per deployment and
   independent of the store: publish(scope, ver) and subscribe(scope).
   In process (one instance), Postgres LISTEN/NOTIFY, Redis or NATS, or
   polling the store's next key (any store; per-request cost, up to the
   poll interval of latency). An instance subscribed to a scope wakes its
   live connections, each of which keeps only changes to its principal's
   view (the digest, std/changes/ui.md 2026-11-16).
5. The browser protocol does not change: Up, Down, the offline queue,
   prediction, confirmed Msgs, `same`, the digest, tab state in the URL.
   Versions are the store's, so they mean one history on every instance.
6. Hot scopes. A scope whose appends conflict often may be leased to one
   instance (a lease record in the store, renewed; others forward its
   syncs). This is an optimization inside the Host, measured before it is
   built.
7. Real-time rooms (a later stage, for games at 20 to 60 updates a
   second): a scope leased to one instance and kept in memory, stepping
   on a fixed tick, taking Msgs and broadcasting state deltas over the
   websocket in a binary encoding instead of HTML, and writing a snapshot
   to the store every few seconds and at the end, not every Msg. The
   client predicts with the same update (Replica), renders to a canvas,
   and the server reconciles. Lobbies, accounts and results stay ordinary
   scopes in the same App.

## Consequences

- Instances are stateless and may stop, crash or redeploy at any time;
  the store holds everything a client's next sync needs.
- One store write per sync that carries Msgs: about 1 to 5 ms in
  Postgres, 20 to 100 ms and about $5 per million writes in S3. Fine for
  carts, forms, dashboards and turn-based games; real-time games use
  stage 7's rooms.
- A cold instance loads a scope from its snapshot and log tail; the
  existing compaction (keep) bounds the tail.
- One page shows one scope until a later stage lets a view combine
  scopes (a shared catalog with a per-user cart). That stage replaces
  decision 0672's user state, whose log per principal is a scope here;
  0672's tab state (the page's query, `with_url_view`) is unaffected.
- The S3 store needs Xo's AWS SDK to send `If-None-Match` on PutObject
  (to check), and a test against real S3 needs the owner's yes; a local
  S3-compatible server comes first.

## Plan

1. The Store and Bus interfaces, the memory store and the in-process bus;
   the Host on them, one scope.
2. A convergence property test with two or more Hosts on one store and
   each sync routed to a random Host, with lost, repeated and reordered
   messages: every replica converges, no Msg is applied twice, no client
   is taken by another principal. Today's Host fails it.
3. Scopes in the Host (a scope function, a cache of active scopes with
   eviction), and examples/ui-shop with a catalog scope and a scope per
   user.
4. The Postgres store and bus, and the directory store; measured with 2,
   20, 200 and 2,000 users over several instances (syncs per second,
   latency, conflicts).
5. The S3 store with the polling bus.
6. Views over two scopes (0672's user state).
7. Real-time rooms, with a small multiplayer canvas game measured for
   input-to-screen latency, ticks per second per room, bytes per second
   per player and CPU per room.

## Stage 1 (2026-11-16)

Built: `Store` (append, read, snapshot, put_snapshot), `memory_store`,
`Bus` and `local_bus`, `Scope` (a Server kept in step with a store:
catch up, run the change, append after the version read, on a lost race
restore and run again), the Host on a Scope, store and bus (defaults: a
memory store and a bus of its own, one instance as before).

Evidence: the convergence property test with 2 to 4 Scopes on one
store, each sync routed to a random Scope, with loss, reordering,
offline clients, refusals and compaction, converges in 30 runs; with a
store per Scope (separate instances, today) it fails. Two Hosts on one
store and bus: a Msg through one reaches the other's live connection and
pages, a retry through the other is not applied again, a client keeps
its principal, server Msgs go both ways, versions match. A cart add in
examples/ui-shop costs what it did (10 to 14 us, one sample).

Found while building it:

- A snapshot's version can fall inside a batch (compaction counts
  entries, a batch holds several), so `read` returns every batch that
  ends past the version asked for and catch-up skips the entries it
  has.
- A new instance must read the store once before it trusts the bus
  (it has heard nothing yet).
- Store functions carry effects. A memory store performs `sync`; a
  Postgres or S3 store performs `net` (a directory store `fs`). Declaring
  them on Store spreads `net` and `fs` to every Host caller, and a test
  using `net` runs only with `--real`, so most std/ui tests would stop
  running by default. Done in stage 1: Store and Bus
  take an effect parameter (`Store[fx]`, as jsonrpc's Conn does), Scope
  carries it, and the Host's effects are inferred from its Scope field;
  `host` (memory) stays `sync`, `host_with(app, store, bus)` performs
  what the store and bus perform.

## Alternatives

- One process (today): no scale beyond one machine, and no failover.
- Sticky sessions: shared data still diverges across instances.
- A database transaction as the only model (Turbo, htmx, LiveView):
  scales, but drops the offline queue and prediction 0659 is built on.
- One writer per room with routing as the only model (Durable Objects):
  every scope needs a router and failover; here it is an optimization
  (6) and the shape of real-time rooms (7).
- A store with SQL transactions as its interface: excludes S3, GCS,
  Azure Blob and a directory; the conditional create is the one
  operation every durable backend has.
