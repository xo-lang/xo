# 0258: discarding the result of a call that only computes a value

Status: Proposed (Xo Lang 2, 2026-10-17; for the owner, through the
lead). Not implemented until accepted.

## Context

`xs.reverse()` returns a reversed copy (spec/std.md). Written as a
statement, it compiles and does nothing. The checker accepts it because
"a non `Result` value may be discarded as an expression statement" (core
2.6). In compiler/arm64 fieldlicm.xo it left a list backward, and the
symptom was a module verification failure far from the cause
(gotchas/list-reverse-returns-copy.md).

The trap is general. Value semantics make every collection and string
method that "changes" a value return a new one: List sort, sort_by,
reverse, map, filter, concatenation; Str trim, replace, to_upper; Map and
Set operations that return a copy. Agents trained on languages where
`list.reverse()` and `list.sort()` mutate in place (Python, Java, C#,
Ruby's bang methods) write exactly this statement. Xo already refuses the
same mistake for errors: an unused `Result` is error XO0201, and a silent
discard needs `.ignore_err("reason")`.

## Decision (proposed)

1. New code XO0209, **error**, from the checker: an expression statement
   whose value comes from a call that only computes a value, discarded.
   Message: "the result of `xs.reverse()` is discarded; it returns a new
   List and does not change `xs`". Fix: when the receiver is a `var`
   binding, `xs = xs.reverse()`.
2. Which calls count. Two parts, so the rule needs no list kept by hand:
   - Prelude and std methods marked as pure in std (an attribute the
     checker reads, `#[pure]` or the doc comment's existing marker,
     whichever the std surface already has). Initially every List, Str,
     Bytes, Map and Set method that returns a value of the receiver's
     type or a derived one and takes the receiver by value: reverse,
     sort, sort_by, map, filter, concat, slice, trim*, replace, to_upper,
     to_lower, split, join, and similar.
   - A user function or method whose effects (section 6) are empty, whose
     parameters are not `var`, and whose result is not `()`: it can do
     nothing but compute the value. Effects already say this, so no new
     annotation is needed.
3. Discarding on purpose: `let _ = xs.reverse()`. It is accepted for
   these calls (unlike a `Result`, there is no failure to justify), so a
   deliberate discard is still visible and greppable.
4. Not covered: calls with effects (writing, sending, spawning), methods
   with a `var` receiver (they change it, so the statement does
   something), and `Result` values (XO0201 and XO0206 already apply).

## Consequences

- One more error for code that today compiles. In a corpus check before
  acceptance, count hits in std, compiler/ and examples: each hit is
  either a bug or a needless call. `xo fix` applies the `xs = xs.f()`
  rewrite where the receiver is a `var` binding.
- Error rather than warning, for the same reason as XO0201: a warning
  agents learn to ignore does not stop the bug, and the statement never
  has an effect.
- The spec's line "a non `Result` value may be discarded as an
  expression statement" gains the exception; spec section 10 gains
  XO0209.

## Alternatives

- A warning (vet only). Cheaper to adopt, but the statement is never
  useful, and warnings are skipped.
- Make reverse and sort mutate a `var` receiver in place. That changes
  the std surface and value semantics for one family of methods, and
  leaves map, trim and the rest with the same trap.
- A hand-kept list of methods in the checker. It drifts from std. The
  effects-based rule covers user code too.
