# 0478: A type's own JSON form and internally tagged enums

Status: Accepted as proposed (owner, 2026-11-03, through the lead): A1
(the typed proxy), B1 (`tag:` and `content:`), N1 (variant wire names),
S2 (SqlRow unchanged). Xo Lang 2 implements it after the HTTP client pool
item, with its measurement plan. Stage 1, variant wire names (N1 and
`case:` on enums), shipped 2026-11-03 (golden json_variant_names); next
the tagged form (B1), then the own form (A1), then the measurements.

## Context

The backend survey of 2026-10-30 (std-coverage.md, "Survey of backend
gaps (2026-10-30)", rank 7, awkward, size M) hit two JSON shapes that
`derive Json` cannot produce:

- **A type's own form.** The ledger service has a decimal `Money` that
  must travel as `"12.34"`. `derive Json` encodes a struct as an object
  and a newtype as its value (core 2.8, decision 0097); nothing lets a
  type choose another form. Every DTO carries amounts as `Str` and
  converts by hand (bench/survey-2026-10-30/ledger/main.xo:198). Go's
  answer is `json.Marshaler` and `json.Unmarshaler`
  (`MarshalJSON() ([]byte, error)`, `UnmarshalJSON([]byte) error`), and
  `encoding.TextMarshaler` for a value that is a string.
- **Internally tagged enums.** The chat hub's messages are
  `{"type": "say", "room": "r", "text": "hi"}`. `derive Json` encodes a
  variant with fields as `{"Say": {...}}` (externally tagged; spec/std.md
  5), so the service decodes into one struct with every field optional and
  a `Str` kind (bench/survey-2026-10-30/chathub/main.xo:27-60). Go has no
  tagged unions; Go services use a custom `UnmarshalJSON` or a two-pass
  decode through `json.RawMessage` and a switch on the type field.

What exists to build on:

- Users cannot implement `Json`: it comes from `derive Json` and the
  builtin types only (spec/std.md 1). Interfaces are structural, and
  there are no associated functions or `Self` constructors, so a decode
  hook cannot be a method called on a type.
- The native, LLVM and wasm codecs are generated per type in
  compiler/ssa/json.xo (`json_enc` and `json_enc_body` from line 240,
  `json_dec_body` from line 788, enums at lines 1104 to 1150 and
  `dec_variant` at 1156); the Go backend's in compiler/gen/json.xo
  (`gen_json_enum` at 224, `gen_json_newtype` at 318). Wire names and
  the `case:` argument are computed once by the checker (decision 0277).
- The decoder reads a parsed tape (std/native/llrt/jsontext.xo): an
  object's keys can be looked up in any order, so a tag key need not come
  first, unlike Go's streaming two-pass.
- Decision 0277 item 5 left enum variant names in JSON unchanged and
  said renaming them is a separate decision: this one.
- Valued enums (decision 0269) give field-less variants a `Str` or `Int`
  value, which is already their JSON.
- `derive SqlRow` allows only Int, Float, Str, Bool, Time, Bytes, their
  options, and `List[Int]` or `List[Str]` fields (spec/std.md 10.4).

## Decision (proposed; options below)

### Part 1: a type's own form, through a proxy type (option A1)

A struct, enum or newtype may declare the JSON type it travels as and
two module functions that convert to and from it:

```
pub struct Money { minor: Int, scale: Int }
  derive Json(via: Str, to: money_text, from: parse_money)

fn money_text(m: Money) -> Str { ... }                  // "12.34"
fn parse_money(s: Str) -> Money ! MoneyErr { ... }      // MoneyErr: Display
```

- `via` is any `Json` type (`Str`, `Int`, a struct, a `List`).
- `to` is `fn(T) -> P` and `from` is `fn(P) -> T ! E` with `E: Display`,
  where P is the `via` type, both resolved in the declaring module (new
  codes for a missing or mistyped one).
- Encoding a T is encoding `to(v)`. Decoding a T decodes a P at the same
  path, then calls `from`; an `Err(e)` becomes
  `DecodeErr{path, msg: e.display()}`, so `at $.legs[2].amount: invalid
  amount "12.3.4"` points at the value.
- The type then works everywhere a `Json` type does: fields, `List[T]`,
  `Map[Str, T]`, `T?`, `Patch[T]`, `json.to_value`, `r.json[T]` in std/sql,
  and std/toml, which decodes through JSON.
- `via` excludes every other `derive Json` argument on that type. It is
  this decision's Go `Marshaler` and `TextMarshaler`, typed: the encoder
  never sees bytes, so it cannot produce invalid JSON (gotchas
  json-marshaler-escapes-html.md is the class of bug a bytes hook allows).

### Part 2: internally tagged enums (option B1)

```
enum Inbound {
  Join(room: Str)
  Say(room: Str, text: Str)
  Ping
} derive Json(tag: "type", case: snake)
```

encodes `Say(room: "r", text: "hi")` as `{"type":"say","room":"r","text":"hi"}`
and `Ping` as `{"type":"ping"}`.

- `tag: "KEY"` puts the variant's name under KEY, beside the variant's
  fields. A variant whose single field is a struct (`from`, or a lone
  unnamed field) puts that struct's fields beside the tag. A variant
  whose payload is not an object (an `Int`) is a new checker error at the
  derive, since there is nowhere to put the tag.
- A variant field, or a wrapped struct's field, whose key equals the tag
  is the 0277 duplicate-key error XO0411.
- Decoding finds KEY anywhere in the object (the tape lookup), selects
  the variant, and decodes the variant's fields from the same object with
  the struct rules (defaults, `T?`, unknown keys ignored). A missing tag
  and an unknown name are `DecodeErr`s at the object's path.
- Variant wire names (0277 item 5): `case:` on an enum's `derive Json`
  converts variant names and their fields' keys; a variant may name its
  own, `Say as "say"(room: Str, text: Str)` (option N1 below), which
  always wins. The same works without `tag:` for the external form.
- `tag: "KEY", content: "KEY2"` (adjacent tagging,
  `{"type":"say","data":{...}}`) is the same code with one more level;
  proposed in the same change, since it is one more level of the same
  code and serde offers both (`#[serde(tag, content)]`).

### Interaction with derive SqlRow

- Part 1: a field whose type has `via: P` is not a SqlRow column by
  that alone (option S2 below). A database column and a JSON value can
  want different forms (`NUMERIC` read as `Str` against `"12.34"`, or
  minor units as `BIGINT`), and SqlRow's allowed field types stay what
  10.4 lists. Reading a `json` or `jsonb` column with `r.json[T]` already
  uses Part 1 with no change.
- Part 2: SqlRow is structs only and decodes no enums; nothing changes.
  A tagged enum stored as `jsonb` is read with `r.json[T]` and gets the
  tagged form.

## Options

Own form:

- **A1, proxy type with named functions (proposed).** Typed, composes
  with every Json container, the generated code is one extra call around
  the proxy's existing codec, and the error carries a path. Costs a new
  derive argument form (function names) and the proxy value's allocation
  (for `Str`, one string per value; Go's `MarshalJSON` allocates its
  bytes too).
- **A2, Go's Marshaler through json.Value.** Methods
  `fn (m: Money) to_json() -> json.Value` and a decode function
  `from: fn(json.Value) -> Money ! E`. Any shape, including one that
  depends on the value; but every value builds a `Value` tree, and the
  checker cannot see the shape (OpenApi, when it comes, could not
  describe it).
- **A3, a streaming writer and a tape cursor.** `encode_json(v, var w:
  json.Writer)` and `decode_json(c: json.Cursor) -> T ! DecodeErr`.
  Fastest and fully general; exposes the tape and the writer as public
  API, which then cannot change, and lets a hook write invalid JSON (the
  Marshaler bug class).
- **A4, bytes hooks as in Go** (`fn (m) marshal_json() -> Bytes`).
  Familiar; needs a validation pass on every output (Go's
  `compact` after MarshalJSON) and loses the decode path.

Tagged enums:

- **B1, `tag:` and `content:` on the derive (proposed).** Matches serde
  (`#[serde(tag = "type")]`, `#[serde(tag, content)]`) and the shapes
  TypeScript and OpenAPI discriminators use.
- **B2, an untagged form** (try each variant in order): ambiguous, slow
  on failure, error messages name the last variant tried. Not proposed.
- **B3, nothing new; document the two-pass decode** with `json.Value`
  and a match. What the services do today, by hand, per message type.

Variant names:

- **N1, `Variant as "name"`** before the payload, mirroring 0277's
  field form. Readable beside the variant, one line per variant.
- **N2, only `case:`.** Covers snake and camel; not irregular names
  (`"chat.message"`).
- **N3, a rename map on the derive.** Rejected for fields in 0277 for
  readability; the same holds here.

SqlRow:

- **S1, reuse the Json proxy for columns.** One declaration, but ties a
  column's form to the JSON form; wrong whenever they differ.
- **S2, nothing now (proposed); a column proxy later** as its own
  derive argument (`derive SqlRow` fields of a type with
  `derive SqlValue(via: P, to, from)`), when a service needs it. The
  ledger's amounts are `BIGINT` minor units, read as `Int` today.

## Measurement plan (before any default or acceptance claim)

All on the first linux/arm64 host with `ab-host.sh` (`--ab`), the
prototype against its parent, plus Go twins under bench/perf/json:

1. **Own form cost.** 100,000 ledger entries, each with two `Money`
   amounts, encoded and decoded: (a) today's workaround (`Str` fields and
   hand conversion), (b) A1 with `via: Str`, (c) A2 through `json.Value`,
   (d) Go with `MarshalJSON` and `UnmarshalJSON` on a decimal type.
   Expect (b) within noise of (a); report (c) and (d) as ratios; peak
   memory for all four.
2. **Tagged decode cost.** 1,000,000 chat messages of five variants,
   about 200 bytes each: the external form today, B1 with the tag first,
   B1 with the tag last, and Go's two-pass (`json.RawMessage` then a
   switch) and one-pass custom `UnmarshalJSON`. Expect tag position not
   to matter (tape lookup) and B1 within noise of the external form.
3. **Code size.** native `__text` of a program with a 20-variant tagged
   enum against the same enum external, and of the ledger DTOs under A1
   against the workaround.
4. **Correctness, every backend.** Goldens for: round trips; a `from`
   error's path and message; a missing tag, an unknown tag, a tag after
   the fields; a tag colliding with a field (the checker error);
   `case:` on variants; `Patch[T]` and `T?` of a `via` type; `r.json[T]`
   on a tagged enum; the Go backend's output equal to native's
   (TestGoldenGoParity).
5. **No compiler effect.** The compiler's own sources use `derive Json`
   (compiler/cli/core/explain_data.xo, compiler/cli/dev/errorcheck.xo
   among others); the self build and fixed point must stay byte-identical
   (`scripts/identity`), since no existing type changes form.

## Consequences

- Money-like values and tagged messages decode into idiomatic Xo types,
  with errors that name the field; the survey's ledger and chathub lose
  their Str workarounds.
- New derive arguments (`via`, `to`, `from`, `tag`, `content`) and the
  `Variant as "name"` form change the parser (N1), the formatter, the
  checker (new XO codes; existing codes keep their meaning), both JSON
  generators (compiler/ssa/json.xo, compiler/gen/json.xo), spec/core.md
  2.3 and 2.9, spec/std.md 5, `xo lsp` hover, and the habit check (Go's
  `MarshalJSON` method could suggest `via`).
- The own form is chosen by the type, not per field: two APIs that want
  one type in two forms need two types (a newtype each), as in serde
  without `with`.
- `OpenApi`, when built, can describe A1 (the proxy's schema) and B1 (a
  discriminator); A2 to A4 it could not.

## Alternatives

The options above; the proposal is A1, B1 with `content:`, N1, S2.
