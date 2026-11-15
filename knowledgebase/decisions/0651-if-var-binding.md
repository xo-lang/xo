# 0651: `if var` and `while var`, a mutable binding in a refutable pattern

Status: Proposed (2026-11-15, aws-43 for the lead; a follow-up of the
owner's language survey, for the owner's approval).

## Context

`if let Some(x) = opt { }` binds `x` immutably; `if var x = opt` is a
parse error today (XO0160 "expected an expression, found keyword
`var`"). Code that mutates a value held in an Option, the common case
for state machines kept in a struct, writes three lines for one idea:

```
if let Some(s0) = q.sv {
  var s = s0
  tls13srv.feed_quic(s, data)
  q.sv = Some(s)
}
```

std/native/llrt/tlsquic.xo (decision 0644 T14) has this five times; the
first draft wrote `if var s = q.sv` at every site and failed to parse.
The write back (`q.sv = Some(s)`) is the part that is easy to forget:
without it the mutation is silently lost, as values are copied.

## Decision

Option A is recommended; B is the owner's call.

- **A (recommended): `if var p = e` and `while var p = e`**, with every
  pattern form and the Option shorthand of `if let` (core 2.5, decision
  0137), bind the pattern's names as `var` bindings: a copy, mutable in
  the block, as `var x = e` is. Nothing is written back. Pure syntax:
  the parser takes `var` where it takes `let`, and the checker marks the
  names mutable. The example becomes

  ```
  if var s = q.sv {
    tls13srv.feed_quic(s, data)
    q.sv = Some(s)
  }
  ```

- **B (in addition to A): write back for a place.** When `e` is a place
  (a `var` binding, a `var` parameter, or a field path of one) and the
  pattern is the Option shorthand or `Some(x)`, the block's end stores
  `Some(x)` into the place on every normal exit (falling off, `break`,
  `continue`, `return`), as a `var` receiver's in-out semantics do. The example loses its last line. A fault does not write back.
  This is less obvious to read (an assignment with no `=`) and needs
  rules for `e` changed inside the block (a new error: the place may not be
  assigned in the block), so it is offered separately.

- `match` arms keep `let` semantics (`Some(var x) =>` is not part of
  this decision).
- `xo fmt` keeps `var`; `xo vet` suggests `if var` for the three line
  form (`if let Some(a) = e { var b = a ...`), as XO0906 does for `??`.

## Consequences

- A is additive; no program changes meaning. The parser, checker and
  formatter change; spec core 2.5 and the `while let` paragraph add
  `var`.
- Tests: check tests for the forms (Option shorthand, `Some(x)`, a
  tuple and a variant pattern, `while var`, an unused-mut warning when
  the binding is never mutated), goldens for the copy semantics (the
  source is unchanged after the block).
- B, if accepted, adds the write-back lowering and its tests (each exit
  kind, a fault, the place-assigned error).

## Alternatives

- **Keep the three line form.** It works, and the survey's cost was
  real but small per site; the lost write back is the stronger reason
  for B.
- **Methods on Option (`opt.update(fn(var x) { ... })`).** A closure
  cannot `return` from the enclosing function or `break` its loop, and
  closures capture by value; rejected as the general answer.
