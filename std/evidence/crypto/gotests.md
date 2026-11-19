kind: gotests
module: crypto
result: pass
commit: 4f6d486a7
source: 0bb9581e1e0e92aae4091774a4d522235926786826e204d3f31bbbbe79c11e03
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" crypto
measure: 249 cases from 8 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/crypto/gotests/go) and the module (std/evidence/crypto/gotests/main.xo), every result equal, error text included.

- crypto/md5 md5_test.go golden (md5): 31 cases, 0 skipped
- crypto/sha1 sha1_test.go golden (sha1): 32 cases, 0 skipped
- crypto/sha256 sha256_test.go golden (sha256): 31 cases, 0 skipped
- crypto/sha256 sha256_test.go golden224 (sha224): 31 cases, 0 skipped
- crypto/sha512 sha512_test.go golden224 (sha512_224): 31 cases, 0 skipped
- crypto/sha512 sha512_test.go golden256 (sha512_256): 31 cases, 0 skipped
- crypto/sha512 sha512_test.go golden384 (sha384): 31 cases, 0 skipped
- crypto/sha512 sha512_test.go golden512 (sha512): 31 cases, 0 skipped

- arm64: 249 cases, 0 mismatches, 8ms
- llvm: 249 cases, 0 mismatches, 6ms
