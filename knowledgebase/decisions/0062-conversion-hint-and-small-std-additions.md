# 0062: Numeric conversion hint on XO0601, `Duration.from_secs_int`, `Response.text`

Status: Accepted (2026-10-08)

## Context
Gaps 7 and 8 of `bench/results/real-app-2026-10-08.md`: the notes-service
tests repeated `resp.body.to_str() ?? ""`, and `Float.from(n)` (a Rust
habit) failed with a bare XO0601 although the conversion `Float(n)` exists
(std.md 2.1). `Duration.from_secs` takes only a `Float`, so whole seconds
from an `Int` needed `Float(n)` or `1s * n`.

## Decision
- **XO0601 hint.** When an associated function that does not exist is
  called on a numeric prelude type (`Int`, `Float`, the sized types) with
  exactly one positional argument whose type is numeric, the diagnostic
  gains the note ``convert with `T(x)` `` and, first in its list, the fix
  that replaces `T.name` with `T`. The code, message, span, and the
  existing "did you mean" notes and fixes are unchanged (fields only
  added). The argument is type checked for this (the failed call did not
  check its arguments before), which may report errors inside it.
- **`Duration.from_secs_int(s: Int) -> Duration`** (std.md 2.6): the same as
  `1s * s`, faulting on overflow. Kept next to `from_millis` and
  `from_nanos`, which already take `Int`.
- **`fn (r: Response) text() -> Str?`** (std.md 4.3): the body as text,
  `None` when it is not valid UTF-8, a streamed body collected first (like
  `decode_json`).

## Consequences
- `xo fix` turns `Float.from(n)` into `Float(n)` because the fix comes first
  (gotcha `xo-fix-applies-suggestion-fixes.md`: the loop applies the first
  fix of each diagnostic).
- Two spellings for whole seconds exist (`1s * n` and `from_secs_int(n)`).
  Accepted: agents look for a named constructor first, and the operator
  form stays the shortest.

## Alternatives
- A generic numeric parameter on `from_secs` (`from_secs[N: Num](s: N)`):
  Xo has no numeric constraint interface, and adding one for this is out of
  proportion. Rejected.
- Making `Float.from` an alias of the conversion: a second way to write
  every conversion. Rejected; the hint teaches the one form.
- A new diagnostic code for the conversion case: the problem is still an
  unresolved name, and agents key on XO0601. Rejected; the hint is extra
  fields on XO0601.
- A hint for a `Str` argument (`Int.from("3")` to `Int.parse`): `parse`
  returns an Option, so the edit would not type check in place. Left out.
