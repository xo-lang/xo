# 0659: local-first UI on one pure update

Status: Accepted (owner, 2026-11-15). Stages 1, 2 and 3 landed the same day: 1a 02aa907a9, 1b 518a68b1f, 1d 3b56feaa0, stage 2 ea90ba87a, stage 3 343b92236 (the browser predicts with the App's own wasm); deterministic update by decision 0662 (24c9a9173)

## Context

Most backends grow a front end, and today that means a second language
and a framework that is rewritten every few years. The churn has one
root: each generation (jQuery, SPA, SSR, hydration, islands, server
components) moves where code runs and how state syncs, and that choice
is baked into the application code, so a change of strategy is a
rewrite. A first sketch gave Xo four rendering modes (static, live over
a websocket, islands, full wasm); the owner asked for one mode that does
not trade any of them away and does not fail on spotty mobile networks.

Xo already has what such a front end needs on the server side: std/http,
std/websocket, html/template with autoescape (0408), a wasm backend
(0179) that runs in the browser (the playground, 0242), derives, and
effects in signatures (core 6) that can prove a function pure.

Evidence (bench/results/ui-local-first-2026-11-15.md):

- **Reconciliation, native simulation.** A todo app with a 5 seat
  resource; one pure update on 4 to 8 clients and a server; the network
  drops (up to 60%), delays, reorders and duplicates packets, and
  clients go offline (up to 83% of all actions were taken offline). 200
  seeds per profile: every replica converged to the server's model,
  with no divergence at any log version and every Msg applied exactly
  once. A negative control (one client's update drifted: a star counts
  double) was caught at the first affected entry and never converged.
  1.5% to 14% of a client's own actions were overturned by the server
  (an edit to a removed item, a seat taken while offline). Without
  server acks clients resent unconfirmed Msgs up to 277x; with an
  `acked` sequence in each reply, at most 8.6x.
- **Visual feedback for agents.** Three fresh agents fixed 10 pages with
  one planted visual bug each: HTML plus screenshots, a new layout
  vocabulary plus a computed text wireframe, and that plus geometric
  diagnostics. All three passed 10/10 on hidden graders with similar
  tokens (71k to 76k). The vocabulary was learned from one page, but it
  did not beat screenshots on these tasks.

## Decision

1. **One application model.** A UI is `Model`, `Msg`,
   `update(model, msg) -> Model` and `view(model) -> Html`. `update` is
   pure: the checker refuses clock, randomness, I/O and any capability in
   it (an effect error with its own code); effects are returned as
   commands whose results come back as Msgs.
2. **One rendering mode: local-first with server reconciliation.**
   - First load: the server renders full HTML with the serialized model
     in the page; it reads before any code runs and works without
     JavaScript.
   - Resume, not hydrate: the client takes the embedded model; views
     that cannot produce a Msg ship as HTML only (islands inferred by the
     compiler, never marked by hand).
   - Every action is a Msg, applied locally at once, kept in a durable
     queue and sent when a connection exists. The server applies each
     (client, seq) exactly once in arrival order, appends it to a log,
     and replies with the log the client has not confirmed plus the last
     seq it applied for that client. The client replays the log with the
     same update and folds its pending Msgs on top (rebase).
   - Offline is a long delay: the app shell is cached, Msgs queue, the
     queue replays on reconnect.
3. **Same code on both sides.** One compiled update ships to client and
   server; the server re-runs every Msg and never trusts client state,
   so validation is not written twice and cannot drift.
4. **Typed exceptions to optimism.** A Msg that needs the server's answer
   (payment, booking, uniqueness) is marked confirmed; the compiler
   requires the view to render its pending state. Fields that must merge
   rather than follow server order declare it with std types (last
   writer wins, counters, text CRDTs).
5. **Server-only data by type.** Model fields marked `server` are never
   serialized to a client; a client-side read is a compile error. Large
   data is a typed query subscription (partial replication), not the
   whole model.
6. **Plain web output.** Views render to plain HTML and CSS; no npm, no
   bundler, the runtime versioned with the language; a typed `use js`
   escape hatch, and views exportable as custom elements so they can
   live inside an existing app.
7. **Visual feedback: screenshots now.** Agents check pages with
   screenshots at phone, tablet and desktop widths. Browser-computed
   checks (overflow, truncation, tap targets, contrast, alignment), as
   the experiment's graders did, can be added later as a tool with no
   language change. A layout vocabulary with a computed text wireframe
   and diagnostics is deferred until an experiment shows it beats
   screenshots.

Staging (each stage its own decision or task):

- Stage 1: `std/ui` as a library, no compiler change: Model/Msg/update/
  view, server rendering on html/template, the Msg log and reconciliation
  protocol over std/websocket with acks, a JavaScript client runtime.
- Stage 2: the purity check on update and the confirmed-Msg pending rule.
- Stage 3: client update compiled to wasm (or JavaScript), bundle size
  and time to resume measured in a browser.
- Stage 4: `server` fields, query subscriptions, persisted queue,
  per-Msg authorization.

## Consequences

- A front end in Xo is a function over values, testable like backend
  code: replay a Msg log and compare against a golden. The reconciliation
  property test is the model for std/ui's own tests.
- Spotty networks and offline use are handled by the model, not by each
  application.
- Bundle size is the main open cost: interactive views ship update and
  their views to the client. Stage 3 measures it before the client
  runtime is fixed.
- Overturned predictions are visible to users (1.5% to 14% in the
  simulation); applications must design for them, and the confirmed-Msg
  rule makes the important ones explicit.
- Not yet covered by evidence: partial replication, server-only fields,
  a persisted queue across reloads, per-Msg authorization, more than one
  server process, and any browser measurement.

## Alternatives

- **Several rendering modes** (static, live websocket, islands, full
  wasm) as build flags: rejected; each mode trades something away, live
  mode fails on spotty networks, and local-first covers what each
  offered.
- **Server-driven live view only** (Phoenix LiveView style): small
  client, but every interaction needs the network.
- **Full client SPA in wasm** (Blazor style): large downloads, slow first
  paint, and a separate API layer that drifts from the server.
- **Adopt an existing JavaScript framework**: keeps the churn and a
  second language, and loses shared types and validation.
- **A layout vocabulary with a compiler-computed text wireframe and
  geometric diagnostics** as the visual feedback loop: built and tested
  (10 tasks, three agents); it tied screenshots, so it is deferred, not
  rejected.
