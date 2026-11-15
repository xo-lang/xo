# 0662: deterministic pure function values

Status: Proposed (owner session, 2026-11-15). Draft only: the owner
decides, and Xo Lang 1 builds, before any compiler change lands. Builds on
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

## Consequences

- std/ui's `App` needs no rule of its own; its `fn(M, E) -> M` type
  already says pure.
- The breaking set is small and local: closures given a pure callback
  type that capture shared state. The experiment's function value column
  (1 in compiler/, 19 in bench/) is the estimate; the real count comes
  from building B, since the experiment flagged calls, not captures.
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
