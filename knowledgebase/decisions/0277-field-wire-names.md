# 0277: Field wire names (`field as "key"`)

Status: Accepted (owner, 2026-10-20). Implemented by Xo Lang 2 (2026-10-20).

## Context

`derive Json` uses each field's name as its key (spec/std.md 5), and
nothing can rename one. Xo Lang Lib 1's service survey (decision 0276)
hit this on third-party payloads: a Stripe event's `"type"` key cannot
be a field because `type` is a keyword, and camelCase or irregular keys
force field names that break Xo's naming. The same holds for the other
derives that map fields to outside names (SqlRow's columns, decision
0061; Toml; Csv).

Two shapes were weighed: a rename list on the derive
(`derive Json(keys: {kind: "type"})`) and a name beside each field. The
owner (2026-10-20): for JSON heavy work and large structs, seeing the
key next to its field is much better to work with.

## Decision

1. A struct field may name its key on the wire, between its name and
   its type:

   ```
   pub struct Event {
     id: Str
     kind as "type": Str
     pub api_version as "apiVersion": Str
     created as "Created": Int = 0
   } derive Json
   ```

   `as` is already a keyword. The name goes before the `:` because after
   the type, `= 8080 as ...` would read as part of a default expression.
2. The wire name applies to every derive that maps field names to
   outside names: Json keys, SqlRow columns, Toml keys, Csv headers (the
   implementer confirms the list against std and records it in the
   spec). In Xo code the field is still `kind`; only the derived
   encoders and decoders use `"type"`.
3. `derive Json(case: snake | camel | pascal | kebab)` (and the same
   argument on the other derives in item 2) converts every field without
   its own wire name; a field's `as "key"` always wins.
4. Errors: two fields of one struct with the same wire name (after the
   case rule), an empty wire name, and `case:` with an unknown value are
   new checker errors with new XO codes (spec section 10; existing codes
   keep their meaning). A wire name on a struct without a derive that
   uses it is a warning.
5. Enum variant names in JSON (`"Variant"`) are unchanged; renaming
   them is a separate decision if needed.

### Implementation (Xo Lang 2, 2026-10-20)

- The derives of item 2, confirmed against std and the compiler: `Json`
  (keys) and `SqlRow` (columns). There is no Toml or Csv derive: std/toml
  decodes into a struct through JSON (std/native/toml.xo `decode[T:
  Json]`), so TOML keys follow the wire names with no code of their own,
  and std/csv maps no structs. `OpenApi` and `Proto` generate nothing yet
  (core 2.9). `case:` is accepted on `derive Json` and `derive SqlRow`.
- The checker computes each field's JSON key and SqlRow column once
  (check decl.xo wire_keys: FieldInfo.json_key, sql_key); every backend
  reads them (compiler/ssa json.xo for the native, LLVM, and wasm
  backends; compiler/gen json.xo for Go), and the field index stays the
  field's name.
- Codes: XO0411 two fields with one key under a derive the struct has,
  XO0412 an empty wire name, XO0413 an unknown derive argument or case
  value or an argument on another derive, XO0414 (warning) a wire name no
  derive uses. A wire name with interpolation is a parse error
  (XO0160).
- The Go habit check (a Go field with a struct tag) now suggests the
  json tag's key as the field's wire name.
- Folded in (lead, 2026-10-20, found by Lib 1): a newtype has its
  underlying value's JSON (core 2.8, decision 0097), which the checker and
  the backends did not implement (a newtype field of a `derive Json`
  struct was XO0404).

## Consequences

- Third-party payloads with keyword, camelCase, or irregular keys
  decode into idiomatic Xo structs, with each key readable beside its
  field.
- The parser, the formatter (`xo fmt` keeps `as "key"` in place), the
  checker, every derive's code generation on every backend, `xo lsp`
  (hover shows the wire name), and spec/core.md 2.2 and spec/std.md 5
  change together.
- Agents editing a large struct change one line per field.

## Alternatives

- **A rename list on the derive** (`derive Json(keys: {...})`): no new
  field syntax, but the keys sit away from their fields, which reads
  badly in large structs (owner).
- **General attributes** (`@json("type")`): familiar from Go's tags,
  but adds an attribute system Xo does not have, for one use.
- **Raw identifiers** (`` `type` ``): fixes keywords only, not
  camelCase or irregular keys.
