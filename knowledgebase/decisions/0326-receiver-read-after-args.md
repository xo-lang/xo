# 0326: A by value receiver is read after the call's arguments

Status: Accepted (owner, 2026-10-21, from the language poll); implemented in
the SSA builder (arm64, LLVM, wasm) and the Go backend. Amends 0142 (a
`var` receiver is read after the arguments) to the other receivers.

## Context

`t.size(t.add(v))` with a by value `size` and a `var` `add`: the call read
`t` before `add` ran, so `size` saw the table without the item (core 5.2,
gotchas/receiver-read-before-args.md). Decision 0142 had made a `var`
receiver and `var` arguments read after every argument, because the old
order dropped the arguments' changes; the by value receiver kept the "like
any operand, first" order. It surprised the compiler's own authors four
times (2026-10-10 to 2026-10-21: the SSA builder, the checker's
`identical(subst(..))`, three more sites) and was found by faults only when
the argument grew a table the receiver indexed; XO0537 (2026-10-21)
reported 19 more sites. The language had two receiver rules for one call
shape, and the one that read earlier was the one nobody expected.

## Decision

A call evaluates its receiver, when that receiver is a variable or a field
path of one (`t`, `t.items`, `(t)`), after its arguments, and reads its
value then. Any other receiver (a call, an index, a literal) is evaluated
first, in source order, since its evaluation has effects of its own.
`t.size(t.add(v))` sees `t` with the item.

The rule applies to every method call that reaches the general path:
methods of the program's types (generic ones too) and calls through an
interface value. The Go backend orders the operands the same way and
puts each non trivial argument in a temporary first, since Go leaves the
order of a variable's read and a call open. The standard library's own
List, Map, Str, and similar methods keep the order they have (their
lowerings read the receiver themselves); the cases that matter, a std
method whose argument changes the receiver's own variable, are `var`
methods (`xs.push(f())`, 0142) or rare.

XO0537 is retired: its shape is now defined, so a note about it would
mislead. The code stays reserved (spec section 10: a code is never reused).

## Consequences

- No program that does not change a variable in an argument of a method
  called on it changes behavior. The compiler itself was built with the new
  rule (fixed point identical); the 19 former XO0537 sites now do what they
  read like, so none needed a rewrite.
- Spec core 5.2, Changes in 0.10, and the gotcha are updated.
- The golden var_recv_after_args covers a variable, a field path, and an
  interface value on every backend.

## Alternatives

- **Keep the order, report it** (XO0537): a warning on a trap the language
  could remove; 19 hits in the compiler alone.
- **Read the receiver after the arguments for every receiver**: moves the
  evaluation of a call or index receiver after the arguments, which changes
  the order of their effects; no code asked for it.
- **Make the arguments' changes an error** (the checker rejects
  `x.m(x.n())`): rejects the common table building shape (0142's reasoning).
