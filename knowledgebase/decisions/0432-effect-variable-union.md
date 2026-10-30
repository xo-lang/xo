# 0432: An effect variable is the union of its arguments' effects

Status: Proposed (Xo lang 5, 2026-10-30, for the owner's review; the
lead assigned it; Xo Lang 1 agreed on the reading). No checker change
until it is accepted; the change is built on a branch so it is ready.

## Context

Core 6.2: "An effect variable is instantiated at each call from the
argument's effects, and may be empty." It does not say what happens when
one variable appears in several parameters. The checker takes it from
the first argument that names it and checks the others against that
set.

Repro (found writing `backoff.retry_notify`, 2026-10-30):

```xo
fn retry_notify[T, E, fx: Effects](b: Backoff, clock: Clock, rand: Rand,
    op: fn() -> T ! E uses fx,
    retryable: fn(E) -> Bool uses fx,
    notify: fn(E, Duration) -> () uses fx) -> T ! E uses (clock, rand, fx)

// in a test: a pure op, a retryable that reads the clock
backoff.retry_notify(b, os.clock, rand, fn() -> Int ! Fail { Err(Fail.Again) },
  fn(e: Fail) { let _now = os.clock.now(); e == Fail.Again },
  fn(_e: Fail, _d: Duration) {})
```

is `XO0701: the expected function type fn(Fail) -> Bool is pure, but
performs clock`. Had the declaration listed `retryable` before `op`, the
same call would compile. The caller cannot fix it: the order is the
declaration's, and adding an effect to `op` to widen the variable would be
a lie. retry_notify now declares one variable per callback (fx, rx, nx),
which works but should not be required.

## Decision

An effect variable is instantiated at each call as the union of the
effects of every argument whose parameter type names it. Core 6.2 gains:

> When one effect variable appears in several parameters, it is the
> union of the effects of all of those arguments.

Why the union: 6.2 already has subsumption, so a callback that performs
fewer effects fits where more are allowed. The union is therefore the
least effect set that every argument fits, and it is what the function's
`uses (..., fx)` charges the caller. Nothing escapes: the caller must be
allowed every effect any of its arguments performs, exactly as when each
callback has its own variable.

Explicitly written effect arguments (`f[T, E, net.dial](...)`) are
unchanged: each argument must fit them.

## Not breaking

The change only accepts programs that fail today; no program that checks
today changes. Today a call checks only when every later argument's
effects are a subset of the first argument's (the instantiated set).
Then the union of all of them is the first argument's set itself, so the
inferred variable is the same set as today at every call that compiles:
the function's `uses`, a result type that names the variable (a returned
closure `fn() uses fx`, a struct with an effect parameter), and what the
caller passes on are unchanged. Codegen does not read effects, so no
behavior changes. Calls that fail today with XO0701 for this reason only
(a later argument with effects the first lacks) compile, with the wider
set charged to the caller, who still needs it in its own `uses` (or gets
the usual XO0701 there).

## Consequences

- spec/core.md 6.2: the sentence above, and an example with two callbacks
  of different effects.
- compiler/check: where an effect variable is bound from the first
  argument, a later argument widens the binding (a set union) instead of
  being checked against it; arguments are still checked against an
  explicit effect argument. The diagnostics at the caller (XO0701 for an
  effect its `uses` lacks) are unchanged.
- Tests: a check test with two callbacks of different effects sharing
  one variable (compiles; the caller's `uses` must list both), the same
  with the caller missing one (XO0701 at the caller, naming it), and
  the repro above. A run of every golden and the fixed point confirms no
  program changed (the inferred sets are the same; codegen ignores them).
- std: retry_notify keeps its three variables (no reason to churn an API;
  they are also a little more precise in a signature dump).

## Alternatives

- **Specify the first-argument rule.** Keeps the checker as it is. The
  result depends on parameter order, the error names a callback that is
  not wrong, and the caller cannot fix it. Rejected.
- **An error that names the conflict** ("fx is pure from `op` but
  `retryable` performs clock: give them separate effect variables").
  Better than today's message, but it still rejects a sound program and
  pushes the work onto every std and library author.
- **One effect variable per parameter, as a lint** (`xo vet` warns when a
  variable names two parameters). Avoids the language change, but the
  lint fires on correct signatures like `Mutex.with`-style helpers that
  pass one callback's effects through two positions, and the union is
  what authors already expect.
