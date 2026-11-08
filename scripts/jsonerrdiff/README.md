# scripts/jsonerrdiff

Differential for ADR 0553 (Go worded JSON decode errors): what `encoding/json` says and what std/json says for the same inputs.
Not a test tier; a measurement that informs the owner's answers.

- `main.go`: Go oracle. 233 inputs over three types (Outer with nested struct, list, map, pointer and uint8 fields; Wrapper;
  Hard with an `UnmarshalJSON` field): every prefix of one document (truncation), syntax errors, one wrong typed value per field,
  several errors in one document, nulls, duplicate and case folded keys. Writes `cases.jsonl` (`go run . > cases.jsonl`; Go 1.26.1).
- `xo/main.xo`: the same types as `derive Json(match: fold)` structs; `xo build -o xotwin ./scripts/jsonerrdiff/xo`,
  `xotwin cases.jsonl > xo.out`.
- `cmp/main.go`: `go run ./cmp cases.jsonl xo.out` classifies each case (same text, Go text as a suffix, different words, Go ok
  and Xo error, ...) and prints examples.

Findings: knowledgebase/decisions/0553-json-go-errors.md. Last run: 2026-11-08.
