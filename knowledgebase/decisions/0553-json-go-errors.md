# 0553: Go-worded JSON decode errors as an opt-in mode

Status: Proposed (Xo Lang 3, 2026-11-07; raised by goversioninfo's FEEDBACK.md items 5 and 6 and Xo Lang 2's differential, 1,920 cases)

## Context

`json.decode` and `json.parse_value` fail with `DecodeErr{path, msg}` whose display is `at $.path: invalid JSON: ...`. Go's
`encoding/json` words the same failures differently:

1. A syntax error: Xo `at $: invalid JSON: invalid character 'x' looking for beginning of value`; Go (Unmarshal) says
   `invalid character 'x' looking for beginning of value` (the prefix is Xo's, documented in spec/std.md 5).
2. A truncated document: Xo `at $: invalid JSON: EOF` (and `unexpected EOF` for a value cut inside a token); Go says
   `unexpected end of JSON input`.
3. A type error: Go's `UnmarshalTypeError` reads `json: cannot unmarshal string into Go struct field Inner.items.n of type int`
   (measured with Go 1.26.1, see "Differential findings": the innermost struct's type name, then the path of JSON keys, with the
   embedded type's name for a promoted field, `Wrapper.Base.id`, and the Go type); Xo's reads `at $.items[0].n: expected integer`
   with the Xo type words.
4. Error order: Go keeps the first `UnmarshalTypeError` and goes on decoding; an error returned by a field's `UnmarshalJSON`
   (Xo: `derive Json(via:)`'s `from`) stops the decode and wins over the saved one.

Programs that port Go tools and compare messages (goversioninfo, the Azure and AWS SDK differentials) rewrite the texts by hand
(goversioninfo's `syntax_msg` and `Dec`). A Go-worded mode in std removes that code and makes the differentials byte exact.

## Decision (proposed)

Add an opt-in `json.GoErrors` mode, off by default so existing programs and goldens keep today's texts:

- `json.decode_go[T: Json](data: Bytes) -> T ! DecodeErr` and `json.parse_value_go(data)`: the same decoders with
  `DecodeErr.msg` in Go's words for cases 1 to 3 and `DecodeErr.display()` equal to Go's `err.Error()` (no `at $:` prefix, no
  `invalid JSON:` prefix). A type error names the field path as Go (1.20 and later) writes it: the JSON keys, not Go
  field names, so the wire name of each field joined by dots after the innermost struct's type name; the Go type words come from a
  fixed table (`int`, `int64`, `string`, `bool`, `float64`, `uint8`, `[]T`, `map[string]T`, a struct as `main.Name` in Go's own
  words, which a port cannot know: the table would give the Xo type name).
- Error order (case 4): in this mode the derived decoder keeps the first type error and continues; a `via` `from` error aborts
  and wins, as Go.
- The default `decode` keeps `at $: invalid JSON: ...` and its stop-at-first-error order; nothing changes for it.

## Notes from Xo Lang 2 (2026-11-07)

- Case 4 (Go's error order) is already on main inside decision 0550 (1928d9add) for `derive Json(match: fold)` types: on any
  error the decoder jumps to a cold block (compiler/ssa/json.xo `dec_fields_fold_err`) that decodes every selected member and
  keeps one error: a hard error (a member whose type is a via type, `json_hard`) wins over type errors, otherwise the earliest
  member by tape index, and a missing field comes after both; the Go backend does the same through `rt.JsonFoldErr.Note`. One
  documented gap: a via type inside a nested non via struct counts as a type error there, as `DecodeErr` has no hard flag. The
  Go-worded mode reuses that block and its rule; only the message builders differ.
- goversioninfo FEEDBACK.md item 8: `DecodeErr` does not name the refused value (`number 1.5`, `string`); that is this
  decision's wording.
- Item 7 is outside this decision as written: Go treats `null` into a non pointer field as a no-op (`{"Major":1,"major":null}`
  leaves 1) where Xo's derive fails with "expected integer". If the mode aims at byte exact Go differentials it should take
  Go's null rule too; to be asked of the owner with the rest.

## Differential findings (2026-11-08)

`scripts/jsonerrdiff` (Go oracle, Xo twin, classifier; Go 1.26.1) runs 233 inputs through `encoding/json` and through std/json
(`derive Json(match: fold)`, the closest mode): every prefix of one document, syntax errors, one wrong value per field, several
errors in one document, nulls, duplicate and case folded keys, an embedded struct, an `UnmarshalJSON` field.

| class | cases |
| --- | --- |
| both fail, Xo's text ends with Go's (syntax errors with a position) | 24 |
| both fail, truncated document: Go `unexpected end of JSON input`, Xo `EOF` or `unexpected EOF` | 123 |
| both fail, truncated inside a literal or number: Go `invalid character ' ' in literal true (expecting 'r')` and kin | 10 |
| both fail, type error: same position, different words | 50 |
| both fail, other (`{a:1}`: Xo omits `looking for beginning of object key string`; 2 `via` rows where Go's `UnmarshalJSON` sees a raw string or null) | 3 |
| Go decodes, Xo fails | 20 |
| Go fails, Xo decodes | 0 |
| both decode, same value | 3 |

What the corpus shows:

1. **A text rewrite after the fact (`go_text()`) covers the syntax errors and most truncations.** 24 syntax cases already carry
   Go's text as a suffix, so stripping the `at $: invalid JSON: ` prefix is exact for them. All 123 plain truncations map to one
   string. It is not exact for the 10 truncations inside a literal or number: Go's scanner is fed a trailing space and names the
   literal (`in literal null (expecting 'l')`, `after decimal point in numeric literal`, `in exponent of numeric literal`), which
   Xo's `unexpected EOF` does not distinguish. Those need the input bytes and the failure offset; `DecodeErr` has neither.
2. **Type errors need more than a rewrite.** Go's text is `Struct.path.to.key` (innermost struct type, then JSON keys) plus the
   Go type and, for a number that does not fit, the literal (`number 1.5`, `number 300`, `number 1e999`); a string into an
   int says only `string`. `DecodeErr` holds the full path as `$.items[0].n` and a message with no Go type name, so the innermost
   struct's name and the Go type words (`main.Inner`) are lost. A mode that produces them must run inside the decoder, as the ADR
   proposes.
3. **Error order already matches.** In every multi-error and `UnmarshalJSON` row (`{"u":300,"name":1}`, `{"n":"x","v":8}`,
   `{"n":"x","v":7}`, ...) Xo and Go report the same member, because `match: fold` (decision 0550) keeps Go's order. Case 4 needs
   no new work, only Go's words.
4. **The larger gap is not wording: 20 inputs Go accepts, Xo refuses.** 8 are a missing field (Go leaves the zero value; a Xo
   struct field without `?` or a default is required), and 12 are `null` into a non pointer field (Go: no op; Xo: `expected
   string`, `expected object`, `expected array`, `expected integer`, `expected boolean`, `expected number`; this includes
   `[null]` into a list of structs, and `{"name":"a","NAME":null}`, where Go keeps `a`). A byte exact differential on valid
   inputs therefore needs Go's null rule and Go's zero filling, not only Go's words (goversioninfo FEEDBACK item 7).
5. **Embedded structs cannot be mirrored.** `derive Json` has no inline option, so `Wrapper{Base; x}` was flattened in the twin;
   Go's `Wrapper.Base.id` path has no Xo equivalent.

## Questions for the owner

1. **Go's null rule.** Should `null` into a non optional field be a no op in a Go mode (as in Go), or stay an error? The 12 null
   cases above are the whole cost; making it a no op only under the Go mode keeps today's behavior by default. Recommendation:
   yes, under `decode_go` only.
2. **Zero filling.** Should the Go mode also accept a missing field and leave the zero value (8 cases)? Without it a Go port
   still has to wrap every struct with defaults. Recommendation: yes, same flag, but ask, since it changes what "valid input" means.
3. **First step.** Ship `DecodeErr.go_text()` first (exact for 24 syntax errors and 123 truncations; wrong for 10 literal or number
   truncations and for the 50 type errors), or go straight to `decode_go` (exact on all
   classes, a decoder change in compiler/ssa/json.xo and std/rt/json.go)? Recommendation: `go_text()` first, with the literal
   truncations fixed by recording the failure offset in `DecodeErr`, and `decode_go` after, so the ports stop hand translating now.
4. **Embedded structs.** Is an inline option for `derive Json` wanted (a separate decision), or does the Go mode map the embedded
   case only through flattened field lists? Not needed for the first step.

`go_text()` itself waits for the owner's answer to question 3.

## Consequences

- Native and Go backends: the decoder generated by the compiler (compiler/ssa/json.xo) takes a flag that selects the message
  builders and the error order; the Go backend's runtime (std/rt/json.go) already holds Go's own texts and can return them
  directly. Cost: one extra flag parameter in the decode entry, no cost on the default path.
- The differential corpus against `encoding/json` (scripts/jsonerrdiff, above) is the proof, rerun with `decode_go` as the
  twin's decoder; goversioninfo's 1,920 cases are a second check.
- Spec 5 gains a "Go worded errors" paragraph; `std/changes/json.md` an entry when it lands.

## Alternatives

- **Always Go wording.** Breaks every golden and program that reads today's `at $.path:` texts, and loses the path in the
  message; rejected.
- **A `DecodeErr.go_text()` method** (the default decode keeps its order and only the text is rewritten after the fact):
  covers cases 1 to 3 cheaply but cannot give case 4 or the innermost struct name, which the failed decode has already
  discarded; could be a first step.
- **Leave it to the ports** (status quo): every port writes the same translation again.
