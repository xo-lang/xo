# 0328: `{}` where a Set is expected is the empty Set

Status: Withdrawn (owner, 2026-10-21). The feature was implemented on a
branch and never merged; instead, `XO0401` for an empty `{}` where a Set
is expected notes "an empty Set is `#{}`" with a fix that writes it
(errorcheck empty_set). `{}` keeps one meaning, the empty Map.

## Context

`#{}` is the empty Set literal (core 2.7), but a `Set[Str]` field default,
parameter, or variable written `{}` (the spelling of an empty Map, and of
the empty value of the other collections' literals `[]`) failed with
`expected Set[Str], found Map[_, _]`. Writers reached for `{}` and `Set[T]()`
(2026-10-21 poll).

## Decision (withdrawn)

An empty `{}` whose expected type is `Set[T]` would have been the empty
Set, in the checker (map_lit with the hint), the SSA builder and the Go
backend. With no expected type, or a Map or anything else expected, `{}`
would have stayed an empty Map. `#{}` stays the context free spelling.

`Set[T]()` is not added: a type is not callable (XO0407), and `List[T]()`
and `Map[K, V]()` do not exist either.

## Consequences

- None in the language: the case stays a type error, and the error now
  names the spelling that works.

## Alternatives

- **The context dependent `{}` (this decision as proposed)**: withdrawn;
  one literal meaning a Map or a Set by context is a second rule to learn
  for one saved character, and the diagnostic covers the habit.
- **`Set[T]()`**: see above.
