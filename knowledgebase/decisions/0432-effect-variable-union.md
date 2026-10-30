# 0432: An effect variable is the union of its arguments' effects

Status: Accepted (owner, 2026-10-30, through the lead; ships in 0.36 after the seed cut, on top of the effect soundness fix, gotchas/effect-unify-unchecked.md). Was: Proposed (Xo lang 5, 2026-10-30, for the owner's review; the
lead assigned it; Xo Lang 1 reviewed it, and the variance and order
sections answer that review). No checker change
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
effects of every argument that reaches it covariantly: the variable is
a parameter's own `uses` (a callback's effects, `op: fn() -> T uses
fx`). Core 6.2 gains:

> When one effect variable is the effects of several function
> parameters, it is the union of the effects of those arguments.
> Elsewhere (an effect argument of a type, `Cache[K, V, fx]`, or a
> function a parameter takes, `register: fn(fn() uses fx)`) the
> variable is bound exactly, and a conflict is an error.

Why the union: 6.2 already has subsumption, so a callback that performs
fewer effects fits where more are allowed. The union is therefore the
least effect set that every argument fits, and it is what the function's
`uses (..., fx)` charges the caller. Nothing escapes: the caller must be
allowed every effect any of its arguments performs, exactly as when each
callback has its own variable.

Explicitly written effect arguments (`f[T, E, net.dial](...)`) are
unchanged: each argument must fit them.

Variance. Only the covariant position widens:

- **Covariant** (a function parameter's own effects): union, above.
- **Contravariant** (`register: fn(fn() uses fx)`: fx bounds what the
  callee will hand the argument): the sound combination of several such
  arguments is the intersection, not the union. This decision does not
  add it; the variable is bound exactly from the first and a later
  conflict stays an error, as today.
- **Invariant** (`cache: Cache[K, V, fx]`, the effects written as a type
  argument): stays equal; a mismatch stays an error, as today.

A variable reached both covariantly and in another position is bound
exactly by the other position; the covariant arguments are then checked
against it (subsumption), as today.

Order. The variable's set at a call is the union after every argument
is checked. A closure literal argument does not read the variable: its
effects are its body's, inferred, and join the union like any other
argument's. The subset obligations (an argument's effects within the
variable) are checked after the whole call (effects.xo's obligation pass),
so an argument checked early is compared with the final set, not the
set at the time it was checked.

## Not breaking

The change only accepts programs that fail today; no program that checks
today changes. Only covariant arguments change, and today such a call
checks only when every later covariant argument's effects are a subset
of the first's (the instantiated set); contravariant and invariant
positions keep today's exact binding, so their calls are untouched.
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
- compiler/check: in infer.xo's argument-against-parameter match (the
  `open_evar(d.eff)` block, about line 205), when the parameter's
  variable was bound earlier in this call (on the trail since the call's
  mark) and the position is covariant, rebind it with `bind_eff(v, ...)`
  to a new set of both sets' items instead of pushing the EffOb
  obligation (the one effects.xo's pass turns into XO0701); unify_eff
  (function type unification) and call.xo's closing of an unbound
  variable to the empty set stay as they are; arguments are still checked against an
  explicit effect argument. The diagnostics at the caller (XO0701 for an
  effect its `uses` lacks) are unchanged.
- Tests: a check test with two callbacks of different effects sharing
  one variable (compiles; the caller's `uses` must list both), the same
  with the caller missing one (XO0701 at the caller, naming it), and
  the repro above; the literal first and the literal last (a closure
literal reading the clock before and after a named pure function, same
result either way); a contravariant use (`register: fn(fn() uses fx)`
twice with different effects: still rejected) and an invariant one
(`Cache[K, V, fx]` beside a callback with more effects: still a
mismatch). A run of every golden and the fixed point confirms no
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
