# 0277: Field wire names (`field as "key"`)

Status: Accepted (owner, 2026-10-20). Implementation assigned to Xo Lang 2.

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
