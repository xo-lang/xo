# 0662: deterministic pure function values

Status: Accepted, option B (owner, 2026-11-15: "go with B, build the
prototype"); landed 2026-11-15 (see Landing). Builds on
0004 (effects in signatures), 0332 (no global variables), 0652 (Proposed,
capability effect parameters) and 0659 (Proposed, local-first UI).

## Context

A function with no `uses` clause is pure: it performs no capability
effect (spec core 6.1). The same section says it "may still use
synchronized state (Mutex, Atomic, Chan)". So pure means "does no I/O",
not "gives the same answer for the same inputs".

Decision 0659 needs the second meaning. std/ui runs one
`update(model, msg)` on the server and, from stage 3, on every client; if
the two can disagree, a client predicts a model the server never
reaches. On main (2026-11-15) the checker already rejects an update that
reads the clock or prints (XO0701), but these type-check:

```
let counter = Atomic[Int].new(0)
ui.app(init, fn(m: M, e: E) -> M { M{n: counter.add(1)} }, view)      // accepted
let shared = Mutex[Int].new(0)
ui.app(init, fn(m: M, e: E) -> M { M{n: shared.with(...)} }, view)    // accepted
let ch = Chan[Int].new(cap: 1)
ui.app(init, fn(m: M, e: E) -> M { M{n: ch.try_recv() ?? 0} }, view)  // accepted
```

Shared state has three ways into a function, and Xo already closes two:

- a global: closed by 0332 (XO0416, a constant cannot hold an Atomic);
- a parameter: std/ui's model and Msg derive Json, and `derive Json`
  rejects a field of type `Atomic[Int]` (XO0404, checked 2026-11-15);
- a closure's capture: open. This is the case above.

## Experiment (2026-11-15)

Worktree `sync-effect-exp` (not pushed): a `sync` effect label in
compiler/check (decl.xo `is_effect_label` and the two label lists), and
`uses sync` on the 15 prelude methods of Chan, Mutex and Atomic that
touch the shared value (the constructors stay pure; `Mutex.with` is
`uses (fx, sync)`). `select` and `for x in ch` were not marked, so the
counts below are a floor. Raw output is in
`~/xo-lead/notes/stdui/syncexp/` on the Mac Studio.

The probe: all three impure updates above get XO0701 ("the expected
function type `fn(M, E) -> M` is pure, but performs `sync`"); the pure
update and the clock and stdio cases behave as before.

The cost of `sync` as a declared effect (option A below):

| Tree | XO0701 for `sync` | pub fn with no `uses` | declares `uses` without `sync` | fn value of a pure type | with a machine fix |
|---|---|---|---|---|---|
| compiler/ (28 modules) | 110 in 22 files | 29 | 80 | 1 | 108 |
| bench/ (303 modules) | 130 in 53 files | 49 | 62 | 19 | 111 |
| std, top-level files using them (15 modules) | 180 in 9 modules | 33 | 147 (with the fn values) | not split | not counted |
| examples/ | 0 | | | | |

std's directory modules (http and others) were not scanned: `xo check`
does not check std on its own, and the scan ran `xo test --std-module`
over top-level files only.

Two findings matter more than the counts:

1. **The lock leaks into the API.** A getter that reads a field kept
   behind a Mutex becomes `uses sync`: `amqp.Connection.peer_container_id`,
   `peer_max_frame_size`, `backoff.Ticker.stop`. 33 std pub functions with
   no effects today would gain one, and so would every pub caller of them
   up to the program. That is the `log` story of decision 0156 again, for
   a detail (how a type protects itself) that callers should not see.
2. **Callback types break where determinism does not matter.** 17 of
   bench's 19 function value hits are test HTTP handlers that count
   requests with an `Atomic`: `http.Handler` is the pure type
   `fn(Request) -> Response`. A real handler that uses a pooled database
   connection or a cache behind a Mutex is the same case. The compiler's
   one hit is the linker passing parallel page hashing as a pure function
   (compiler/xo/ld.xo:119): concurrent inside, deterministic outside.

## Decision (proposed)

Option B: **a function value's captures decide, not every call.**

- A closure that captures a value whose type holds shared state (a
  Mutex, Atomic, Chan, Once, Semaphore or task.Group, directly or in a
  field, a list, a map or an enum payload) performs `sync`. A function
  value of a pure function type may not; the error is XO0701, as for any
  other effect, with a note on the capture.
- Named functions do not declare `sync`. It is ambient in a function's
  own body (as std's `log` is, 0156), so a type that locks inside its
  methods keeps its API. A named function can reach shared state only
  through a parameter (0332), and its parameter types already say so.
- `sync` is a label a function type can allow: `fn(Request) -> Response
  uses sync`. http.Handler, websocket handlers and the other std callback
  types whose callers share state on purpose say so in the same change.
- **Interfaces.** A captured interface value hides its dynamic type, so
  the capture rule alone cannot see a Mutex behind it. Under 0652 an
  interface takes its effect as a parameter; a capture of an interface
  value then performs what its effect parameter says, and an
  implementation that locks must be given a `sync` instance. Without
  0652, a captured interface value is treated as performing `sync`
  unless the interface is marked pure. This is the one place B depends on
  0652's outcome.

Then a pure function value means deterministic: same inputs, same
result. std/ui gets its guarantee for update and view, and so does any
other library that replays, memoizes or runs a function on two machines.

## Prototype of B (2026-11-15)

Branch `sync-effect-exp` (commit 4c5c81142, local, not pushed), about 120
lines in compiler/check:

- `use_var` (call.xo): in a closure, a use of a local declared outside it
  whose type holds a Chan, Mutex or Atomic at any depth (`shared_handle`,
  a copy of XO0416's type walk with `is_sync_handle`) performs `sync`,
  with the note "`x` is a captured Mutex: shared state another task may
  change".
- `sync` stays ambient outside closures: `perform` drops it in a declared
  function, a test or an initializer; the effect fixpoint drops it from a
  declared function's inferred set (it can arrive through a closure passed
  to a generic call such as `scope.spawn`); and a Decl or Iface obligation
  ignores it.
- The label is accepted in `uses` clauses and module ceilings.

Probes: the three impure updates of the Context get XO0701 for `sync`;
also a closure capturing a struct with an Atomic field, and an outer
closure that calls an inner one that captures (the note points at the
capture). Accepted: a declared pub function that locks or calls such a
closure, shared state passed as a parameter, `scope.spawn`, and
`Router.handle` handlers (their type takes an effect parameter).

| Tree | Option A | Option B |
|---|---|---|
| compiler/ | 110 | 0 |
| examples/ | 0 | 0 |
| bench/ | 130 | 22 in 11 files |
| std (A: 15 top-level modules; B: all 94) | 180 | 24 in 8 modules |

B's 46 hits are of two kinds, and none has a machine fix yet:

- A callback type that is pure today and is called from another task,
  where shared state is the point: FakeNet's `os.net.handle` handlers
  (`fn(http.Request) -> http.Response`, 15 bench test hits),
  `metrics` `gauge_func` (`fn() -> Float`), a reader built from
  `fn() -> Str? ! IoErr`, std/native/http's body producers, the TLS
  session cache callback, std/trace's middleware handler, a worker pool's
  job type. Each type gains `uses sync` (or an effect parameter, as
  `Router.handle` has) in std.
- A closure with its own `uses` clause that captures shared state
  (std/amqp, amqptest, grpc, backoff, rate; one in bench): its clause
  gains `sync`. A fix that writes it is worth adding.

## Landing (2026-11-15)

The prototype plus:

- **std/native is exempt for now.** The seed compiles std/native, and a
  seed rejects an unknown effect label, so its sites (HTTP body pipes and
  readers, the TLS session cache callback) cannot say `uses sync` yet.
  The capture rule skips native modules, as `log` is ambient there (0156).
  After the next seed cut, decision 0653's sweep removes the exemption
  (`native` in compiler/check call.xo `use_var`) and adds `sync` to those
  types.
- **The closure clause fix.** XO0701 on a closure whose `uses` clause
  misses an effect now carries the same note and fix as a declaration's
  ("declare them: `uses (net.dial, sync)`"), for any effect, not only
  `sync`.
- **std:** `sync` added to twelve closure clauses (amqp, amqptest,
  backoff, grpc, jsonrpc, rate) and to the callback types of FakeNet
  `handle`/`handle_tls`, metrics `gauge_func`/`counter_func`/`collector`
  and the types metrics `handler`/`handler_at`/`middleware` and trace's
  middleware and handler return; amqptest_test's polling helper. std then
  checks clean (TestStdChecks, stdgen -check). The prototype's per-module
  count missed jsonrpc and metrics (`xo test --std-module` of a module
  with no tests checks nothing); TestStdChecks is the complete check.
- **bench:** the three non-paper sites outside std's types (a test
  `ReadLine` alias) fixed; the metrics and FakeNet changes fixed the rest.
  Five sites stay in bench/paper, which holds solutions written blind
  from the spec and is never edited; that tree already has unrelated
  errors from older drafts.
- **No vet code** (decision 0653): 0662 makes no older form obsolete; the
  breaking sites get XO0701's fix where one is mechanical (a closure's
  clause). A capture given a pure callback type has no local fix: the
  callback's type, often in another module, decides.
- Test: test/errorcheck/effect_sync_capture (fails on main before this
  change: no XO0701, and `sync` is an unknown effect, XO0702).

## Consequences

- std/ui's `App` needs no rule of its own; its `fn(M, E) -> M` type
  already says pure.
- The breaking set is small and local, as measured by the prototype: 0
  in compiler/, 24 in std, 22 in bench/, most of them in about seven std
  callback types.
- The checker needs a "holds shared state" walk over types. The XO0416
  check of constants already has one to start from.
- Effect inference over captures sits next to Lang 1's queued fix for an
  XO0703 false positive (an effect arriving through an interface field
  reported unused, xw #38). B should land after that fix and add tests
  for a capture inside an interface field.
- spec core 6.1 changes its sentence on synchronized state; a gotcha
  explains XO0701 for `sync` with the fix (`uses sync` on the callback
  type, or pass the value instead of capturing the handle).

## Alternatives

- **A: `sync` as a declared effect on every operation.** Measured above:
  420 errors in compiler/, bench/ and part of std, 85% with machine
  fixes, and two lasting costs (locks in public signatures, every
  handler type opting in). It also catches a named function that locks a
  parameter, which B treats as visible already. Not chosen; B can grow
  into A if a real case needs it.
- **A std/ui-only rule.** A checker rule (or vet note) for function
  values passed as `App.update` and `App.view`. Smaller still, but a
  special case for one library; the next library that replays a
  function (caching, simulation, a deterministic test harness) needs its
  own.
- **Runtime check only.** std/ui could compare client and server models
  in tests and development builds (0659's reconciliation experiment
  already detects drift). Useful as a second line and kept in either
  case, but it finds the mistake after it ships.
- **No change.** Leaves stage 3 of 0659 with a known way to diverge.
