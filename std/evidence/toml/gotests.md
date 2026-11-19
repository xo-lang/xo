kind: gotests
module: toml
result: pass
commit: aa44ef04b
source: 5ca6c8556034200a28e16c096cc41c7ef286a5602b8d28e0032f1156119389a3
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" toml
measure: 316 cases from 15 tables of Go's tests (go1.26.11, github.com/pelletier/go-toml/v2 v2.4.3), 2 backends (arm64,llvm), 0 mismatches with Go, 11 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/toml/gotests/go) and the module (std/evidence/toml/gotests/main.xo), every result equal, error text included.

- github.com/pelletier/go-toml/v2 unmarshaler_test.go TestUnmarshal_Integers.examples (doc): 11 cases, 0 skipped
- github.com/pelletier/go-toml/v2 unmarshaler_test.go TestUnmarshal_Floats.examples (doc): 19 cases, 0 skipped
- github.com/pelletier/go-toml/v2 unmarshaler_test.go TestUnmarshal.examples (doc): 127 cases, 0 skipped
- github.com/pelletier/go-toml/v2 unmarshaler_test.go TestUnmarshalDecodeErrors.examples (doc): 95 cases, 11 skipped
- github.com/pelletier/go-toml/v2 unmarshaler_test.go TestLocalDateTime.examples (doc): 11 cases, 0 skipped
- github.com/pelletier/go-toml/v2 unmarshaler_test.go TestUnmarshal_RecursiveTable.examples (doc): 5 cases, 0 skipped
- github.com/pelletier/go-toml/v2 unmarshaler_test.go TestUnmarshal_RecursiveTableArray.examples (doc): 5 cases, 0 skipped
- github.com/pelletier/go-toml/v2 unmarshaler_test.go TestUnmarshal_Nil.examples (doc): 2 cases, 0 skipped
- github.com/pelletier/go-toml/v2 unmarshaler_test.go TestDecoderStrict.examples (doc): 4 cases, 0 skipped
- github.com/pelletier/go-toml/v2 errors_test.go TestDecodeError_Position.examples (doc): 4 cases, 0 skipped
- github.com/pelletier/go-toml/v2 errors_test.go TestDecodeError_PositionAfterComments.examples (doc): 6 cases, 0 skipped
- github.com/pelletier/go-toml/v2 errors_test.go TestDecodeErrorRedefinition.examples (doc): 8 cases, 0 skipped
- github.com/pelletier/go-toml/v2 unmarshaler_astgeneric_test.go TestUnmarshalInvalidDateTimeIntoTyped.cases (doc): 4 cases, 0 skipped
- github.com/pelletier/go-toml/v2 unmarshaler_astgeneric_test.go TestUnmarshalTypeMismatchErrors.cases (doc): 9 cases, 0 skipped
- github.com/pelletier/go-toml/v2 unmarshaler_astgeneric_test.go TestUnmarshalRedefinitionIntoStruct.cases (doc): 6 cases, 0 skipped

- arm64: 316 cases, 0 mismatches, 6ms
- llvm: 316 cases, 0 mismatches, 4ms
