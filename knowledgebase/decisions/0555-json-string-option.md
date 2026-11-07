# 0555: The `string` option of a JSON wire name

Status: Proposed (Xo Lang 3, 2026-11-07; requested by xo-lead for Xo Azure 1's GCP generator, which meets tens of thousands of Discovery fields with `format: int64` and similar)

## Context

Go's `json:",string"` writes a scalar field as a JSON string holding the value's literal (`5` as `"5"`, `true` as `"true"`)
and reads it back the same way. Google's Discovery documents use it for every `int64`, `uint64` and some `double` fields (a
JSON number cannot hold a 64 bit integer in a JavaScript client), so a generated Go struct carries `json:"n,omitempty,string"`
on tens of thousands of fields. Decision 0309 gave the wire string Go's tag options after a comma and made `omitempty` the
only one; any other option is `XO0422` ("unknown wire name option"), and 0309 said the next option takes a new code.
Today a generator would have to wrap every such field in a custom type.

What Go does (encoding/json go1.26.6, encode.go `typeFields`, decode.go `object`, `literalStore`, `valueQuoted`):

- The option applies when the field's type (after one pointer) is a bool, any integer, a float or a string; on any other type
  it is silently ignored.
- Encode: the value's normal literal inside quotes. A string field is quoted twice (`"abc"` becomes `"\"abc\""`). A nil
  pointer is `null` (not quoted); `omitempty` is decided on the value before quoting (`0`, `false`, `""` and `nil` omitted).
- Decode: the JSON value must be a string or `null`. `null` is stored as for an unquoted `null` (the field is left as it
  was; an `Option` becomes `None`). A string is parsed as the literal of the field's type: `""` is an error, as is any text
  that is not exactly a JSON literal of that type (`"0x10"`, `"1 "`, `" 1"`, `"true"` into an Int); a number, bool, array
  or object in place of the string is an error ("trying to unmarshal unquoted value into T"). A quoted `"null"` is accepted
  as `null`. An integer out of range is the usual overflow error. For a bool the text is exactly `true` or `false`.

## Decision

1. **Syntax.** The wire string of a field (`as "..."`, or a `json "..."` override of 0315) takes the option `string` beside
   `omitempty`, in either order and with the key part unchanged: `n as "n,omitempty,string": Int?`, `as ",string"` for the
   default key, `json "id,string"`. A `sql` string still takes no options (`XO0422`). A repeated option is `XO0422` (as
   Go's tag parser would not mind, this is stricter on purpose: a generator bug shows up at once).
2. **Types.** `string` applies to `Bool`, `Int`, `Int8` to `Int64`, `UInt8` to `UInt64`, `Float`, `Float32` and `Str`, and
   to an `Option` of one of them (Go's one pointer level). On any other field type it is a new checker error, `XO0431`
   ("the string option applies to scalar fields only"), where Go ignores it. Reason: a silently ignored option is a wire
   format bug found in production; the generator knows the field's type, so refusing costs it nothing. If the owner wants
   Go's silent ignore, `XO0431` becomes a warning with the same text and nothing else changes.
3. **Encoding** (`derive Json`, `json.encode`, `json.to_value`, `json.json_writer`): the same literal the field writes
   without the option, inside quotes; a `Str` is written as the quoted form of its quoted JSON text (Go's double
   quoting); `None` is `null`; a NaN or an infinity is the error the unquoted encoder already gives. `omitempty` tests
   the value as 0309 defines emptiness, before quoting.
4. **Decoding** (`derive Json`, `json.decode`, `json.parse_value` into a type): the value must be a JSON string or `null`;
   otherwise `DecodeErr` with the field's path and the message `invalid use of ,string struct tag, trying to unmarshal
   unquoted value into Int` (Go's words, as 0553 words the rest in its opt-in mode; here there is no mode: this
   error exists only for `string` fields). A string whose text is empty or is not a literal of the field's type is
   `invalid use of ,string struct tag, trying to unmarshal "TEXT" into Int`; an out of range integer is the message the
   unquoted decoder gives for overflow. `null` and the string `"null"` leave a plain field at its default and make an
   `Option` `None`. The text is parsed with the JSON number grammar (no leading `+`, no hex, no surrounding space),
   as Go's `literalStore` does.
5. **Codes.** `XO0422` keeps its meaning (an unknown or repeated option; its table text now names `omitempty` and `string`
   as the known ones). `XO0431` is new (checker; `string` on a non scalar field). No existing code changes meaning;
   `xo explain`, the code table of spec/core.md and spec/std.md's json section gain the entries.
6. **Other formats.** `derive Cbor`, `Yaml`, `Toml` and `Xml` do not take the option (`XO0422`), as they take no
   `omitempty` today unless their own decisions say so. `json.Value` objects are unaffected.

## Consequences

- Azure 1's generator emits `as "n,omitempty,string"` straight from the Discovery `format`; no wrapper types.
- Cost: the encoder and decoder paths for scalar fields get one more branch each, taken at compile time (the derive knows the
  option); fields without it generate the same code as today. The checker gains one check; the std/json corpus gains the
  Go differential below.
- Proof when built: a Go oracle corpus (internal/stdtest json corpus) of every scalar type with and without `omitempty`, `Option`,
  quoted and unquoted inputs, malformed texts and overflow, compared with `encoding/json`; the 0309 omitempty cases rerun
  with the option; a fuzz of the quoted decoder against Go.

## Alternatives

- **A wrapper type per field** (`Int64String` with its own Json impl): rejected, tens of thousands of fields, and every use
  site would unwrap.
- **Silently ignore the option on non scalars** as Go does: kept as the fallback of decision 2.
- **A format wide setting** (`derive Json(int64: string)`): rejected, Google mixes quoted and unquoted integers in one
  struct, and Go's per field tag is what the generator mirrors.
- **Decode leniently** (accept an unquoted number under `string`): rejected, Go does not, and the differential would differ.
