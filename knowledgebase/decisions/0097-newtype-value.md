# 0097: Reading a newtype's value: `e.0` and the pattern `Email(p)`

Status: Accepted (2026-10-09, chosen under the owner's task to close spec
audit gap 4; implemented on the Go backend the same day)

## Context

Core 2.8 declares newtypes, `type Email struct(Str)`, constructed with
`Email("x")`, with the equality, hashing, and JSON of the underlying
value. The spec audit (`bench/results/spec-audit-2026-10-09.md`, gap 4)
found that nothing reads the value back: `e.0` was XO0601 ("no field or
method `0`") and `Email(s)` in a pattern was XO0302 ("variant pattern does
not fit"), so a newtype was usable only for equality and as a map key.
The audit recommended `e.0`, matching tuple fields, plus the pattern
`Email(s)`. No earlier decision covers newtypes (checked `decisions/`).

The Go backend already lowers a newtype to `struct { F0 T }` and its place
resolution already treats a newtype's `.0` like a tuple field, so the gap
was in the checker only.

## Decision

- **Field.** A newtype value has one field, `.0`, of the underlying type
  (with the newtype's type arguments substituted: `Wrap[Int]`'s `.0` is
  `Int`). It reads like a tuple field and, on a `var`, is assignable like
  one (`e.0 = "b"`). Any other index is XO0601 "newtype `Email` has one
  field, `.0`". A function typed `.0` is callable as `w.0(x)`.
- **Pattern.** `Email(p)` (or `mod.Email(p)` for a newtype of another
  module) matches a value of that newtype when `p` matches its `.0`. It
  is irrefutable when `p` is, so `let Email(s) = e` destructures, and
  exhaustiveness treats a newtype as a single constructor with one field
  (a missing case is shown as `Email(_)`). `Email(..)` matches any value.
  A pattern without parentheses, with more than one or a named field, or
  against another type is XO0302.
- **Name resolution.** When the matched value is an enum with a variant
  of the same name, or the name is a prelude variant (`Some`, `Ok`, ...),
  the variant wins; otherwise the pattern path is looked up as a type.
- **Visibility.** Reading follows construction, which is open wherever
  the type is visible: there is no private newtype value. A type that
  must guard its value (a validated `Email`) is a struct with a non `pub`
  field and a constructor function, as before.
- **Native backends** (`--no-gc`) keep reporting XO1201 (planned) for
  newtypes, now including `.0` and newtype patterns. (Superseded the
  same day: the native backends lower a newtype as a struct with the one
  field `0`, so `e.0`, its assignment, `==`, hashing, Map keys, and Debug
  text `Email("a")` work there too; patterns `Email(p)` match the field.
  Goldens `internal/llvm/testdata/golden/newtypes` and `newtype_value`.)

No new diagnostic codes.

## Consequences

- Newtypes become usable for the wrapper role the spec intended
  (`UserId`, `Email`, units): unwrap, destructure, match on the value.
- `e.0` is the only spelling: no `.value`, no `into()`. Agents that know
  Rust's tuple structs (`email.0`, `Email(s)`) write it unprompted.
- Newtypes still give no encapsulation (anyone can build or read one);
  this is unchanged and now stated in core 2.8.

## Alternatives

- **A named method (`e.value()` or `e.unwrap()`).** Adds a method to every
  newtype that can collide with a user method of the same name, and does
  not give patterns. Rejected.
- **Pattern only.** Forces a `let` or `match` to read a value inside an
  expression. Rejected as clumsy; `.0` costs nothing extra.
- **Private value outside the defining module.** Would make newtypes
  encapsulating, but construction is already open, so it would be
  inconsistent; changing construction too is a larger, breaking change
  that no task has asked for. Revisit with a decision of its own if
  needed.
