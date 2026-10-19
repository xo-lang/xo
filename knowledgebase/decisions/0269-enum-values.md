# 0269: Enum values (valued enums, all(), named constants for open sets)

Status: Proposed (Xo Lang 3, 2026-10-18, for the owner's review; the
direction is the owner's, given through the lead the same day). Spec
changes wait for acceptance.

## Context

Xo enums are sum types (core 2.3): closed, matched exhaustively, and
encoded by derive Json as the variant's name (std.md, JSON rules: a unit
variant is `"Variant"`). Three kinds of code need more than that today:

- Closed sets with a wire or storage value: a weekday stored as 1 to 7, a
  log level stored as `"warn"`, a database enum column. Code writes a
  `match` each way by hand, and the two matches can drift.
- Open sets with names: HTTP status codes, TLS alert numbers, HTTP/2
  error codes. A peer may send a value no list names, so an enum (closed)
  is the wrong type. std writes these as untyped `Int` constants and a
  hand-written text table: `tls12.xo` has `pub const ALERT_CLOSE_NOTIFY:
  Int = 0` and so on, `tlsname.alert_text` matches the same numbers again,
  `http1.xo` keeps the HTTP/2 error names as a positional list
  (`["NO_ERROR", "PROTOCOL_ERROR", ...]`), and std/http takes
  `status: Int` everywhere, so `http.json(20, v)` type checks.
- Iterating a closed set (`for d in Weekday...`) has no spelling; code
  keeps a list next to the enum by hand.

`use c` binds a C enum type as `Int32` (decision 0093) and binds none of
its enumerators, so C code that takes an enum is called with bare numbers.

## Decision

### 1. Valued enums (closed sets)

An enum whose variants are all field-less may declare a value type, `Int`
or `Str`, after its name; each variant may then give its value:

```
enum Weekday: Int { Mon = 1, Tue, Wed, Thu, Fri, Sat, Sun }
enum Level: Str { Debug = "debug", Info = "info", Warn = "warn", Error = "error" }
enum Color: Str { Red, Green, Blue }       // "Red", "Green", "Blue"
```

- Values are constant expressions of the value type (literals, `const`
  names, arithmetic on them for `Int`).
- Default numbering: an `Int` variant without a value is the previous
  variant's value plus one, and the first is 0 (C, Rust, Swift). A `Str`
  variant without a value is its own name (Swift).
- Values are unique within the enum: a repeat is a new error `XO0421`,
  which names both variants.
- `e.value()` returns the value (`Int` or `Str`). `E.from(v)` returns
  `E?`: the variant with that value, or `None`. `Weekday.from(8)` is
  `None`; there is no faulting form, since `.from(v) ?? fallback` and
  `.from(v).or_err(...)?` already cover both uses.
- A value type other than `Int` or `Str`, a variant with fields in a
  valued enum, or a value that is not a constant of the value type is a
  new error `XO0422`. An `Int` value outside the 64 bit range is the
  existing integer literal error; there is no narrower range, since the
  value type is `Int`. (`use c` enums, below, are checked against C's
  `int` range when they are bound.)
- The enum stays a closed sum type: `match` is exhaustive by variant,
  equality, hashing, and derived `Ord` are by variant in declaration
  order (not by value, so reordering values never reorders a sort;
  `sort_by(fn(d) { d.value() })` sorts by value), and derived `Display`
  prints the variant name. Only `value()`, `from()`, and JSON change.
- derive Json on a valued enum writes and reads the value: `Weekday.Mon`
  is `1`, `Level.Warn` is `"warn"`. Decoding a value no variant has is a
  decode error that names the enum and the value. A field-less enum
  without a value type keeps today's encoding (`"Variant"`), which is the
  same text as a `Str` valued enum with default values, so adding `: Str`
  to such an enum does not change its wire format.
- Generic valued enums are not allowed (`XO0422`): a valued enum has no
  fields, so a type parameter could not be used.

### 2. `E.all()` on field-less enums

Every enum whose variants are all field-less, valued or not, has
`E.all() -> List[E]`, the variants in declaration order. On an enum with a
field variant it is `XO0601` (no such method), with a note saying why.

### 3. Open sets: named constants of a distinct integer type

An open set is a newtype over `Int` (core 2.8) with module constants:

```
pub type Status struct(Int)

pub const OK: Status = Status(200)
pub const CREATED: Status = Status(201)
pub const NOT_FOUND: Status = Status(404)

pub fn (s: Status) is_success() -> Bool { s.0 >= 200 && s.0 <= 299 }
```

- Note on syntax: the owner's sketch wrote `type Status = Int` (distinct).
  In the spec today, `type X = Int` is an alias, interchangeable with `Int`
  (core 2.8); the distinct form is the newtype `type X struct(Int)`. This
  record uses the newtype and adds no new declaration form. If the owner
  wants the shorter spelling, a follow-up can make `type X = distinct Int`
  sugar for the newtype.
- A newtype construction of a constant is a compile time value, so it is
  allowed in a `const` initializer (core 3.1 already allows struct and
  enum construction; this names newtypes explicitly).
- Constant patterns: in a pattern, a name that resolves to a `const`
  (bare in its own module, qualified as `http.OK` elsewhere) matches by
  equality instead of binding. The risk is a misspelled constant
  (`NOT_FUOND`), which resolves to nothing and so binds a fresh name that
  matches everything (Rust's long-standing trap). A binding pattern
  spelled in the constant style, upper case with underscores, is
  therefore a new error `XO0423`: constants are written that way and
  bindings never need to be, so the typo is caught and the fix (the
  constant's nearest name) is suggested. The
  newtype pattern with a range already works for groups:
  ```
  match resp.status {
    http.OK | http.CREATED => "done"
    http.NOT_FOUND => "missing"
    Status(500..=599) => "server error"
    _ => "other"
  }
  ```
- Exhaustiveness: a newtype over `Int` is never covered by constants, so
  such a `match` needs a `_` or binding arm (the existing `XO0301`). That
  is the point of an open set: the code says what happens to a value no
  name covers.
- std/http moves to `http.Status`: `Response.status: Status`,
  `json(status: Status, v)`, `text`, `empty`, `stream`, the client
  response, and `StatusError.status()`. Constants for the codes Go's
  net/http names (`http.OK` ... `http.NETWORK_AUTHENTICATION_REQUIRED`),
  and methods `is_informational`, `is_success`, `is_redirect`,
  `is_client_error`, `is_server_error`, `text()` (Go's StatusText).
  Migration: `xo fix` rewrites an integer literal argument or field
  initializer of type `Status` to its constant (`200` to `http.OK`), or to
  `http.Status(299)` when no constant names it. No new implicit
  conversion (core 2.10 keeps exactly three).

### 4. `use c` enums

`use c` binds a C enum type `enum mode` as a valued enum
`enum Mode: Int { ... }` with one variant per enumerator (UpperCamel from
the C name, with the common prefix shared by all enumerators dropped:
`MODE_READ` to `Read`). C allows two enumerators with one value; the
first is the variant and each later one a `const` alias of it. An
enumerator outside C's `int` range is reported in the binding's skip list.
At the boundary, a parameter of the enum type takes the Xo enum (the
adapter passes `e.value()`); a result stays `Int32`, since C may return a
value the enum does not name (flag sets, new library versions); the
binding comments it `// Mode.from(r)`. A C enum used as a flag set (values
that are distinct powers of two, combined with `|` in the header's own
macros) is bound as today's `Int32` plus `const` values, not as a valued
enum, since its values combine.

### 5. Lowering

- A valued enum is represented like any field-less enum: its variant tag.
- `Int` values that are 0, 1, 2, ... in declaration order are the tags:
  `value()` is the tag and `from(v)` is a range check. Any other `Int`
  values: `value()` loads from a constant table indexed by the tag;
  `from(v)` uses a table of tags with holes when the values span no more
  than four times the variant count, else a `match` on the value
  (compiled as any `match` on an `Int`).
- `Str` values: `value()` loads from a constant table of static strings
  (no allocation); `from(v)` is a `match` on the string, which the
  compiler lowers by length, then compare (as http1.xo's key_kind does by
  hand).
- `all()` returns a list built once per enum and shared (no allocation
  per call once built).
- Json uses `value()` and `from()`; nothing else in the backends changes.

### 6. std migration

- TLS alerts (an open set: a peer may send any byte): `tls.Alert`, a
  newtype over `Int` with constants, replacing the `ALERT_*` Int
  constants; `tlsname.alert_text` becomes `Alert.text()`, a `match` on the
  constants with a `_` arm.
- HTTP/2 error codes (open, RFC 9113 says unknown codes are not errors):
  `h2.ErrCode` newtype with constants and `text()`, replacing the
  positional name list in http1.xo.
- HTTP status: section 3.
- Closed sets in std that are written as `Int` constants today become
  valued enums where no outside value can reach them; an audit lists them
  in the implementation change.

### New diagnostics

| Code | Meaning |
|------|---------|
| `XO0421` | two variants of a valued enum have the same value |
| `XO0422` | invalid valued enum: a value type other than `Int` or `Str`, a variant with fields, type parameters, or a value that is not a constant of the value type |
| `XO0423` | a binding pattern is spelled like a constant (upper case) but no `const` of that name is in scope: a misspelled constant pattern |

## Consequences

- One declaration gives the name, the value, the reverse lookup, and the
  wire format, so the hand-written pairs of matches go away and cannot
  drift.
- Open sets get a type: `http.json(20, v)` no longer type checks, and a
  `match` on a status must say what happens to an unnamed code.
- Breaking for std/http callers (every `status: Int` site); `xo fix`
  migrates them. `bench/paper/` solutions are not edited (they were
  written blind from the spec), so the ones that pass a status as an
  integer stop compiling; how the benchmark treats them (rewritten blind
  again from the new spec, or measured against the old one) is the
  owner's call when step 3 lands.
- Constant patterns change what a bare name in a pattern means when a
  `const` of that name is in scope: an existing upper case binding with
  the name of a constant in scope would become a comparison. Upper case
  binding names are unidiomatic and rare; the implementation change
  reports every such pattern in the repo and std (a one-time scan by the
  checker, listed in the change) and renames them before the rule takes
  effect, and `xo fix` does the same for users.
- Implementation order when accepted: (1) valued enums, `all()`, Json,
  `XO0421`, `XO0422`, spec core 2.3 and std.md JSON rules; (2) constant
  patterns and newtype consts, core 3.1 and 3.5; (3) http.Status and the
  `xo fix` migration, std.md 4; (4) use c enums, core (C interop); (5) the
  std tables. Each step is usable alone.

## Alternatives

- Swift raw values (`enum Weekday: Int`, `init?(rawValue:)`,
  `CaseIterable.allCases`): the model taken here, with the same default
  numbering and default names. Swift also allows `Character` and floating
  point raw values; Xo keeps `Int` and `Str` (below).
- Rust explicit discriminants (`enum E { A = 1 }`, `e as i32`): one way
  only; the reverse is a hand-written `TryFrom`. Open sets use
  `#[non_exhaustive]` or newtypes with associated consts, and consts in
  patterns compare by equality, as section 3 does.
- Go typed constants (`type Status int; const OK Status = 200` with
  `iota`): every value of the type is legal and `switch` has no
  exhaustiveness, which suits open sets (section 3 is this model with
  required `_` arms) but not closed ones.
- TypeScript string enums (`enum Level { Warn = "warn" }`): value per
  member, no reverse mapping for strings, and the community moved to
  unions of string literals. Xo's `Str` valued enum keeps the closed set
  and adds `from()`.
- Out of scope, recorded for later:
  - Flag sets: a later library type `Flags[E]` over a valued `Int` enum
    (bit per variant), not a language feature.
  - Several constant properties per variant (Java enums with fields): a
    method with a `match` covers it (`fn (p: Planet) mass() -> Float`),
    and the compiler can lower such a match to a table.
  - Other value types (`Float`, `Rune`, sized integers, `Bytes`): no use
    in std or the bench tasks needs them; `Int` covers the sized integer
    cases with a range check at the boundary.
- A fourth implicit conversion (an integer literal where a `Status` is
  expected): rejected to keep core 2.10's three conversions; `xo fix`
  carries the migration instead.
- `as` casts between enums and integers: rejected; `value()` and
  `from()` make the failure case explicit.
