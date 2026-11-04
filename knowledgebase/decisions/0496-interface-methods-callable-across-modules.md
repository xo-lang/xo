# 0496: A method that implements a visible interface is callable by name across modules

Status: Proposed (Xo Lang 2, 2026-11-04, from the AWS agent's feedback log; for the owner to decide)

## Context

Core 2.4: a type satisfies an interface through its methods whether or
not they are `pub`, and calling a non `pub` method by name from another
module is an error (XO0605). The AWS agent wrote `fn (k: Key) display()
-> Str` to satisfy `Display` in one module and called `k.display()` from
another: XO0605. The value can already be used through the interface
(`"${k}"`, a `Display` parameter, `as Display`), so the rule hides
nothing the other module cannot reach; it only makes the direct call
fail. Since 2026-11-04 XO0605 carries a fix that adds `pub`
(compiler/check base.xo pub_fix).

## Decision (proposed)

(b): a method of a `pub` type is callable by name from another module
when it implements a method of an interface that module can name (a
prelude or derivable interface such as `Display`, `Ord`, `Json`, or a
`pub` interface it imports), with that interface's signature. Every
other non `pub` method stays private. Spec 2.4's bullet changes to say
so; XO0605's note names the rule.

## Consequences

- An implementation detail can no longer be private if its name and
  signature match a visible interface's method; a module that wants it
  private names it otherwise, as Go requires an exported name for an
  interface method anyway.
- The checker looks up, at a non `pub` method call from another module,
  whether the method implements an interface visible to the caller: a
  check on the error path only, no cost on valid calls.

## Alternatives

- (a) Keep the rule, with the `pub` fix added on 2026-11-04 (the cheapest;
  every agent hits it once and applies the fix).
- (c) Only the prelude and derivable interfaces (`Display`, `Debug`,
  `Ord`, `Eq`, `Hash`, `Json`): the case the feedback hit, without
  reaching user interfaces.
- (d) Rust's rule, a trait's methods as public as the trait: Xo has no
  `impl Trait for T` block to attach it to, so it reduces to (b).

Recommendation: (c) first (the feedback's case, and the commonest: a
`display` or `cmp` written by hand), widened to (b) if agents hit it with
their own interfaces.
