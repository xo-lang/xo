# 0440: `@internal` std struct fields, and a constant's fix drops the call

Status: Accepted (lead for the owner, 2026-10-30: darwin testing bugs b19
and b22, ~/xo-lead/notes/lead/darwin-feedback-2026-10-30.md). Built by
Xo Lang 2 the same day.

## Context

b19: `uuid.Uuid{hi: 1, lo: 2}`, `u.hi` and a UuidErr's `msg` checked
clean in user code, though spec std 24 says a Uuid's two Ints are not
visible outside std (XO0912 had deprecated the old field in 0.34.0 and
was retired). The report read this as XO0605 not applying to std types.
The cause is std's own convention: std's declaration files write no
`pub`, and the checker makes every std declaration and field public
(compiler/check decl.xo); http.Request's `method` and `path`, for one,
are public API with no `pub`. Applying XO0605 to std's non-pub fields
would hide every std field from user code.

b22: `flag.continue_on_error()` (a `use go` binding) gets XO0601 with
the fix `CONTINUE_ON_ERROR`, a case only rename, and the fix kept the
`()`, so `xo fix` left XO0407 ("CONTINUE_ON_ERROR is not a function").

## Decision

1. A doc comment line `@internal` on a std struct keeps its fields to
   std's modules: user code that names one (a field access, a struct
   literal, a struct pattern, `with`) is `XO0605`, "field `hi` of `Uuid`
   is not `pub`", as for another user module's non-pub field. std's
   modules see each other's fields, `@internal` or not (std/native's
   implementation reads Uuid's words). Other std fields stay public. It
   is the prelude's encoding rule 10b, beside `@move_only` (10a).
2. std/uuid's Uuid is `@internal`. UuidErr's `msg` stays public: spec
   std 24 lists it as `struct UuidErr { msg: Str }`, as other std error
   types expose their text.
3. When XO0601's fix for a module member called with no arguments names
   a constant, the fix replaces the call: `flag.continue_on_error()`
   becomes `flag.CONTINUE_ON_ERROR`, with a note that it is a constant.

## Consequences

- BREAKING for user code that used a Uuid's `hi` or `lo` (0.34.0 to
  0.35.x): use `u.bytes()`, `u.display()`, `uuid.from_bytes`, and `<`.
- A std type that should hide its representation gains `/// @internal`
  rather than `pub` on its public fields.

## Alternatives

- `pub` in std meaning what it means in user modules: every std file
  would need `pub` on its public declarations and fields; a large edit
  for one hidden type, and a std declaration that forgot `pub` would
  vanish from user code.
- Hiding UuidErr's `msg` too: the spec lists it, and `e.display()` gives
  the same text, so hiding it buys nothing.
