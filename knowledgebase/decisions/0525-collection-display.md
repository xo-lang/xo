# 0525: Display for List, Map and Set

Status: Accepted (owner-approved item, 2026-11-05, relayed by the lead)

## Context

`"${xs}"` on a List, Map or Set was XO0404, so agents hand wrote loops to see values, or used `${xs:?}`
(decision 0296) without knowing it. Option and Result stay without Display on purpose (the note tells the
caller to unwrap with `??`); a collection has no such alternative.

## Decision

`List[T]`, `Map[K, V]` and `Set[T]` implement `Display` when every type argument does. The text is the
collection's `Debug` text, the same rule as a derived Display (core 2.9): `[1, 2, 3]`, `{"a": 1, "b": 2}`,
`#{"x", "y"}`, `[]`, in the order the collection iterates (insertion order), elements shown with their
Debug text (a Str keeps its quotes, so `["a", "b"]` and `[a, b]` cannot be confused, and an element with
a comma is unambiguous). Go's `%v` (`[1 2 3]`, `map[a:1]`) is not followed: it loses the quotes and the
brackets differ.

Only the checker needed the change: the native backends already printed a value with no display method
through its Debug text, and the Go runtime's `Display` falls back to `Debug`.

## Consequences

- `${xs}` and `xs.display()` work for collections of Display elements; an element type without Display
  (an Option, Bytes, a struct without `derive Display`) is still XO0404, naming the element type.
- A user type element prints its Debug form, not its hand written `display`, as with a derived Display.

## Alternatives

- Elements through their own Display (`[a, b]` for strings): ambiguous, and different from `${xs:?}`.
- Go's `%v` format: see above.
- Leave it as an error: costs every agent a loop or a lookup.
