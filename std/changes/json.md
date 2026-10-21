# std/json changes

## 2026-10-21: test: parse_value against Go's encoding/json

- Kind: test
- Commit: (filled in after the push)
- Found by: differential test against Go
- Go reference: encoding/json Decoder.Token, scanner errors (go1.26.1)
- Test: std/native/json_test.xo (3,059 texts: encoding/json scanner cases and seeded random ones)

No difference found: the native reader accepts and rejects the same
texts as the Go backend's, with the same error texts.

## 2026-10-21: feature: json.Value, JSON of any shape

- Kind: feature
- Commit: (filled in after the push)
- Found by: survey (dynamic JSON; std/template's data, decision 0299)
- Go reference: encoding/json decoding into `any`
- Test: internal/llvm/testdata/golden/json_value (every backend)

`Value` (Null, Bool, Int, Num, Str, List, Object in key order),
`to_value`, `parse_value`, `encode`, and `get`.

## 2026-10-20: feature: field wire names and derive Json(case: ...)

- Kind: feature
- Commit: (this change)
- Found by: survey
- Go reference: encoding/json struct tags (go1.25.3), as a field's `as "key"` instead of a tag
- Test: internal/llvm/testdata/golden/json_wire_names and sql_wire_names, test/errorcheck/wire_names

A field names its JSON key with `kind as "type": Str` (a keyword or a
camelCase key no longer forces the field's name), and `derive Json(case:
camel)` (or snake, pascal, kebab) converts the other fields' keys
(decision 0277).

## 2026-10-20: fix: a newtype field of a derive Json struct

- Kind: fix
- Commit: (this change)
- Found by: user
- Go reference: none
- Test: internal/llvm/testdata/golden/json_wire_names

A newtype such as `type Email struct(Str)` as a field of a `derive Json`
struct was rejected with XO0404; it now encodes and decodes as its
underlying value on every backend, as core 2.8 says.
