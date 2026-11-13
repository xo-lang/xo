# 0622: Leaving a field out of a derive (`as "-"`, Go's `json:"-"`)

Status: Accepted (owner, 2026-11-12, relayed by the lead; Xo Lang 3
implemented it the same day, at aws-43's request).

## Context

aws-43's gcp SDK generator ports Google's Go client models. Each model
struct carries `ForceSendFields []string` and `NullFields []string`,
tagged `json:"-"` in Go: plain `json.Marshal` never writes them, and only
`gensupport.MarshalJSON` reads them to build a request body. Xo has no
way to say this. Spec core 2.2 (decision 0309, 88b70a440) made `as "-"`
the key `-` for `derive Json` ("Unlike Go"), and every other option is
`XO0422`. The generator would have to drop the fields (and lose the
gensupport behavior) or write them into every encoded value.

The derives disagree today on the same field:

- `derive Cbor` (0527), `derive Yaml` (0512) and `derive Xml` (0315)
  read a field's own format string, else its `as` string, and the name
  `-` leaves the field out (compiler/check/derive.xo, `o.skip`,
  `tag.skip`). Goldens and corpora rely on it: cbor_direct's
  `skip as "-": Str`, the cbor and yaml corpora's `hidden as "-": Int`.
- `derive Json` and `SqlRow` read the same `as "-"` as the key or column
  `-`.
- `derive Proto` (0530) reads only its `proto "N"` tag, and every field
  must carry a number (`XO0422` for a missing one), so a field cannot be
  left out at all.

So `skip as "-": Str` on a struct that derives Json and Cbor is a
skipped field in CBOR and a `"-"` key in JSON. No program in the
repository uses `-` as a JSON key or a column name (`git grep` over
.xo files on 2026-11-12: the only `as "-"` uses are the Cbor and Yaml
tests above).

Go's rule (encoding/json, and the same in encoding/xml, yaml.v3,
fxamacker/cbor and sqlx's `db` tag): the tag `-` leaves the field out;
the tag `-,` is the key `-`.

## Decision

1. **`-` means "leave out" for every derive, as in Go.** A field whose
   wire string for a format (its own `json`, `sql`, `xml`, `yaml`, `cbor`
   string, else its `as` string) is exactly `-` is not part of that
   format: the encoder never writes it and the decoder never reads it (a
   key `-` in the input is unknown, as any other unknown key). The
   string `-,` is the key `-` (Go's spelling); `-,omitempty` is the key
   `-` with omitempty.
   - Shared: `force_send_fields as "-": List[Str] = []` is out of JSON,
     TOML, SqlRow, CBOR, YAML and XML at once.
   - One format: `secret json "-": Str = ""` is out of JSON only; its
     column, CBOR key and XML element stay its name.
2. **A skipped field still needs a value when a decoder builds the
   struct.** The decoder fills it with the field's declared default,
   else the zero its type has (None, false, 0, `""`, an empty List, Map,
   Set or Bytes; Cbor's and Yaml's rule today). A skipped field with
   neither (a struct or enum type without a default) is a new error,
   `XO0440` "a field left out of derive D needs a default", at the
   field. An encode-only use still needs it: the derives generate both
   directions.
3. **`derive Proto`: `proto "-"`.** A field tagged `proto "-"` has no
   number and is not on the wire, under the same default rule (2). Its
   `as` string never affects Proto (Proto reads only `proto`), so a field
   left out of JSON by `as "-"` and kept in Proto writes a number as
   today. A field with no `proto` tag stays `XO0422`: Proto keeps asking
   for an explicit choice per field.
4. **No separate `skip` option.** `as ",skip"` was considered (below);
   one spelling, Go's, is what generators and Go readers expect, and it
   is already what four of the seven derives do.
5. **Diagnostics and tools.** `xo vet` warns (`XO0441`, warning) on
   `-,` with no other option on a struct that derives none of the formats
   that read it, as a likely typo. `xo fmt` keeps the strings as written.
   astjson and defhash already carry the wire string; nothing new.
6. **Breaking change** (spec core 2.2 and the CHANGELOG **Breaking**
   line): a struct that relied on `as "-"` as the JSON key or column `-`
   now loses the field; the migration is `as "-,"`. None in the
   repository, and none in the consumers' code (confirmed 2026-11-12:
   aws-43 found no `as "-"` or `as "-,` over its aws, azure, gcp and
   openai SDKs; scarlet-03 none over scarlet, google-workspace-mcp and
   the cached gcp modules). aws-43's gcp models carry the two lists today
   as the keys `~forceSendFields` and `~nullFields` (its decision 0013)
   and switch to `as "-"` when this lands.

## Implementation (when accepted)

- compiler/check/derive.xo: one helper decides skip from a field's
  format string (Json, SqlRow, Toml through Json, Cbor, Yaml, Xml, and
  `proto "-"`), and `-,...` strips the leading `-` as the key `-`. The
  Json and SqlRow paths gain the skip (SSA json_* lowering and the Go
  generator's struct tags: `json:"-"` and `json:"-,"` map one to one, so
  the Go backend needs only the tag text).
- The default check (2) and `XO0440` in the checker; the spec code table,
  `xo explain XO0440`, `XO0441` (the next free codes on 2026-11-12; recheck at implementation).
- Tests: a golden `derive_skip` encoding and decoding one struct under
  Json, Cbor, Yaml, Xml and Proto (Go parity: Go's encoding/json gives
  the same bytes for the JSON half), errorcheck `derive_skip_default`
  (`XO0440`), json corpus rows for `-` and `-,`.
- Docs: spec core 2.2 (replace "Unlike Go"), 2.9 (Proto), std.md 5,
  CHANGELOG **Breaking**, std/changes/json.md.

## Consequences

- aws-43's generator emits `force_send_fields as "-": List[Str] = []`
  and `null_fields as "-": List[Str] = []` as Go does, and its
  gensupport port reads the fields directly.
- One field means the same thing under every derive; a struct that
  derives Json and Cbor no longer disagrees with itself.
- A program that used `-` as a JSON key changes behavior without a
  compile error (6); the mitigation is the CHANGELOG line, and a search
  of the known consumers before the push.

## Alternatives

- **An option `skip`** (`as ",skip"`, `json ",skip"`), keeping `-` the
  key: no break, but a spelling no Go reader or generator knows, and
  Cbor, Yaml and Xml already skip on `-`, so the derives would still
  disagree unless they also changed (breaking their tests instead).
- **A field attribute outside the string** (`@skip force_send_fields:
  List[Str]`, or a contextual word `skip json`): new syntax for one
  case, and still a second spelling beside Go's.
- **Leave it to the generator** (drop the fields, or a second struct for
  the wire): loses gensupport's ForceSendFields behavior, or doubles
  every model.
- **Only Json** (fix `-` in Json, leave SqlRow and Proto): smaller, but
  SqlRow would then be the one derive where `as "-"` is a name, and
  Proto would still have no way to leave a field out.

## Implementation notes (2026-11-12)

- check: FieldInfo.json_skip and sql_skip (exported with the module), set
  when the json or sql string (else the shared `as`) is exactly `-`; an
  empty option is accepted, so `-,` is the key `-`. A skipped field is
  out of the duplicate key check, the Json field type requirement and
  SqlRow's column type check. XO0440 is reported for Json, SqlRow and
  Proto (`proto "-"`, check derive.xo proto_opts -3, skip_zero_text).
- Lowering: compiler/ssa json.xo (encoder, decoder: default else zero,
  SqlRow), jsonval.xo (to_value), compiler/gen json.xo (Go backend
  encoder, tree and direct decoders, SqlRow; skip_value).
- Yaml: `-,` was already the key `-` when writing, but the decoder used
  the key text `-` as its own skip marker and left the field unread;
  the marker is now `<<skip>>` (found by the golden).
- XO0441 is not added: the case it named (a wire string on a struct
  that derives no format reading it) is XO0414's existing warning.
- Tests: golden derive_skip (arm64, llvm, go parity, wasm; the JSON bytes
  are encoding/json's), errorcheck derive_skip_default, `xo explain
  XO0440`.

