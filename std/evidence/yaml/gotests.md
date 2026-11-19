kind: gotests
module: yaml
result: pass
commit: 72766fc68
source: 5cba69062199850e386bb2a59aa1de0db9fe830d9c2cbb213be8c040ddfdcf81
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" yaml
measure: 532 cases from 7 tables of Go's tests (go1.26.11, gopkg.in/yaml.v3 v3.0.1), 2 backends (arm64,llvm), 0 mismatches with Go, 11 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/yaml/gotests/go) and the module (std/evidence/yaml/gotests/main.xo), every result equal, error text included.

- gopkg.in/yaml.v3 decode_test.go unmarshalTests (node): 168 cases, 4 skipped
- gopkg.in/yaml.v3 decode_test.go unmarshalTests (reenc): 168 cases, 4 skipped
- gopkg.in/yaml.v3 decode_test.go unmarshalErrorTests (node): 16 cases, 1 skipped
- gopkg.in/yaml.v3 decode_test.go unmarshalErrorTests (reenc): 16 cases, 1 skipped
- gopkg.in/yaml.v3 node_test.go nodeTests (node): 74 cases, 0 skipped
- gopkg.in/yaml.v3 node_test.go nodeTests (reenc): 74 cases, 0 skipped
- gopkg.in/yaml.v3 decode_test.go unmarshalErrorTests (json): 16 cases, 1 skipped

- arm64: 532 cases, 0 mismatches, 5ms
- llvm: 532 cases, 0 mismatches, 8ms
