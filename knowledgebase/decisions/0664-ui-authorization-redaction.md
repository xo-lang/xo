# 0664: std/ui authorization and per-principal views (decision 0659 stage 4)

Status: Proposed (owner session, 2026-11-15).

## Context

Decision 0659 (Accepted 2026-11-15) put per-Msg authorization, server-only
fields, query subscriptions and a persisted queue in stage 4. The queue
landed in stage 1 (localStorage, per tab since 343b92236). What std/ui
does today with the rest:

- **Anyone can send any Msg.** `POST _xo/sync` takes an `Up{client, ver,
  msgs}`; the server applies each Msg whose seq is the client's next
  (`Server.receive`). The client id is a random string the browser made;
  nothing ties it to a person. Same-host Origin and the JSON content type
  stop cross-site posts, not a signed-in user sending Msgs they should
  not.
- **Every client sees the whole model.** The page embeds it, sync replies
  carry its view (and the model with `?model=1`), and a `Replica` gets
  the log entries, which hold every client's Msgs.

0659 point 5 states the target: Model fields marked `server` never reach
a client, and a client-side read is a compile error; large data is a
typed query subscription. Two facts shape the first step:

1. `update` is pure and runs on both sides (0659 point 1, decision 0662),
   so it cannot be where a secret is read on the server only; server-only
   work is a command (an effect returned by update, its result a Msg),
   and commands are not built yet.
2. The browser runtime never replays log entries: it sends `?thin=1`
   (no entries) and, when it predicts, folds its own pending Msgs over
   the server's model (`?model=1`). So what a browser learns is exactly
   the model and HTML the server sends it, which the server can shape per
   person without changing the protocol.

## Decision

Stage 4a, a library change with no compiler change:

1. **A principal per request.** `ui.host(app, ..., auth: fn(http.Request)
   -> Str? uses (...)? = None)`. With `auth`, every route calls it; `None`
   is 401 (the page route may redirect through the app's own handler
   instead, by mounting the page itself). The principal is an id string;
   roles and membership live in the model, where `update` and `allow`
   can read them.
2. **A client is bound to its principal.** The server records the
   principal of a client id's first sync; a sync for that id under
   another principal is 403. The runtime treats 403 as "signed out":
   it drops its queue and client id (the Msgs were the old principal's)
   and reloads.
3. **Who may send which Msg:** `allow: fn(M, Str, E) -> Bool = None`
   on the host, pure, run by `Server.receive` before `update`. A refused
   Msg is acknowledged (so the client stops resending it) and not
   applied or logged; the reply counts it (`Down.refused`), and the
   runtime sets `data-xo-refused` for the app's CSS. To the client a
   refusal is an overturned prediction, which 0659 already designs for.
4. **What each principal sees:** `redact: fn(M, Str) -> M = None` on the
   host, pure: every model a client receives (the page's state, a sync's
   `model`, a snapshot) and every view rendered for it is of
   `redact(model, principal)`. A host with `redact` never sends log
   entries (a Msg may carry another person's data); a Xo `Replica` of
   such a host gets snapshots only. Server-only fields are the case
   where `redact` blanks a field for everyone.
5. **Prediction stays correct enough:** the wasm client predicts over the
   redacted model it was given; when the server's answer differs, the
   next sync overturns it, as for any prediction.

Later stages, each its own decision:

- 4b: commands (0659 point 1): `update` returns the model and a list of
  commands; the server runs server commands with their effects and feeds
  the results back as Msgs. With commands, 0659 point 5's `server` field
  modifier (a type whose value is never serialized to a client, readable
  only with a `server` effect that `update` cannot have) replaces
  blanking by `redact`.
- 4c: query subscriptions (partial replication) for models too large to
  send whole.
- Later: more than one server process (the log in a store), merge types
  (last writer wins, counters, text CRDTs), a compiler check that a view
  renders a confirmed Msg's pending state, the app shell cached for an
  offline first load.

## Consequences

- A std/ui app can be put behind sign-in and have per-person data
  without a compiler change, using the app's existing session handling
  (`auth` reads its cookie or bearer token).
- `allow` and `redact` are plain pure functions over the model: tested
  like `update` (std/ui's convergence test gains principals and refused
  Msgs).
- A server field blanked by `redact` is still visible to `update` and
  to server code by mistake (a view could render it to the person
  allowed to see it only because `redact` ran first); the compile-time
  guarantee waits for 4b.
- With `redact`, every sync renders the view per principal: a cost per
  request the convergence and browser benchmarks measure.

## Alternatives

- **A `server` field modifier now (0659 point 5 as written).** Needs a
  checker rule and, for anything useful on the server, commands; 4a gets
  the security property (nothing reaches a client it should not) first.
- **A typed principal `App[M, E, P]`.** Every App gains a type
  parameter; an id string with roles in the model covers the cases seen
  so far (a todo list per person, a shared list with an owner).
- **Authorization inside `update`** (ignore a Msg the sender may not
  send): `update` does not know the sender, and adding it to every Msg
  lets a client forge it.
- **A projection type for clients (`V` separate from `M`).** Two models
  mean two `update`s, which breaks 0659 point 3 (the same code on both
  sides).
