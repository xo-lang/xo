# 0582: an interface value's effect argument widens where it is only covariant

Status: Proposed (Xo Lang 1, 2026-11-09; asked by Xo Azure 1, ask 3 of
~/xo-lead/notes/aws-43/PROPOSAL-std-devcred-asks.md, through the lead).

## Context

A type's effect argument is exact (core 6.2): `Cred[fs.read]` is not a
`Cred[(fs.read, proc)]`, so a credential that reads files cannot be pushed
into a `List[Cred[(fs.read, proc)]]` of credentials that may also run a
process. Today that is XO0701 ("this value's type has effects `fs.read`,
but the expected type `Cred[(fs.read, proc)]` has `fs.read, proc`", the
wording of 139be7659). A concrete struct with fewer effects already
converts (its methods' effects are checked against the interface's), so
only an already boxed value is refused; the workaround is an adapter struct
per credential type.

Widening is sound only where the effect parameter is covariant: a value
whose methods perform at most `fs.read` can be called where the caller
budgets `(fs.read, proc)`. Where the parameter appears in a method's
parameter (`fn run(self, f: fn() uses fx)`), widening would let a caller
hand the value a function that performs `proc` where its own type promised
`fs.read`; a stored field typed with it is invariant.

## Decision

1. Each effect parameter of an interface has a variance, computed once
   when the interface is declared: covariant when it appears only in its
   methods' `uses` clauses, and otherwise exact (any parameter type,
   result type, or other type's argument that names it). Version 1 does
   not look inside result types (a covariant `Box[fx]` result stays
   exact); it can be added later without changing what compiles today.
2. Where a value of `I[e1]` is used as `I[e2]` (a conversion site:
   assignment, argument, return, `List.push`; core 2.10's assignability),
   a covariant parameter needs only `e1` within `e2`. Narrowing is never
   allowed, and exact parameters stay exact.
3. Nothing else widens: `List[I[e1]]` is not a `List[I[e2]]` (a List is
   mutable, so invariant), and type inference, `==` and pattern matching
   keep exact effect arguments.

## Consequences

- Azure 1's credential lists and policy chains hold values of narrower
  effect arguments without adapters.
- The checker records a variance per interface effect parameter and, at
  assignability, checks a covariant argument one way (an XO0701 that
  names the value's effects missing from the expected type) instead of
  both ways.
- spec/core.md 6.2 states the rule and the "Changes in 0.N" list gains
  it; errorcheck tests cover widening (push, argument, return), narrowing,
  a parameter in a method's parameter (exact), and List[I[e]] (invariant).
- Generated code is unchanged: an interface value's representation does
  not depend on its effect argument.

## Alternatives

- Keep exact effect arguments (the adapter workaround): every SDK with a
  credential or policy chain writes one adapter per implementation.
- Declared variance (`interface Cred[out fx: Effects]`, Kotlin's `out`):
  explicit, but a new syntax for what the declaration already shows;
  inference keeps the language smaller and can only allow more programs.
- Widen every effect argument everywhere: unsound for parameters used in
  method parameters and for mutable containers.
