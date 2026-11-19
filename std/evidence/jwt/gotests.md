kind: gotests
module: jwt
result: pass
commit: 6cb948533
source: 7f43aa00b9a27a11b37e51351ee348690d6b035a5e1103fd936c32bbcb93136e
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" jwt
measure: 18 cases from 5 tables of Go's tests (go1.26.11, github.com/golang-jwt/jwt/v5 v5.3.1), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/jwt/gotests/go) and the module (std/evidence/jwt/gotests/main.xo), every result equal, error text included.

- github.com/golang-jwt/jwt/v5 hmac_test.go hmacTestData (verify): 4 cases, 0 skipped
- github.com/golang-jwt/jwt/v5 rsa_test.go rsaTestData (verify): 4 cases, 0 skipped
- github.com/golang-jwt/jwt/v5 rsa_pss_test.go rsaPSSTestData (verify): 4 cases, 0 skipped
- github.com/golang-jwt/jwt/v5 ecdsa_test.go ecdsaTestData (verify): 4 cases, 0 skipped
- github.com/golang-jwt/jwt/v5 ed25519_test.go ed25519TestData (verify): 2 cases, 0 skipped

- arm64: 18 cases, 0 mismatches, 15ms
- llvm: 18 cases, 0 mismatches, 12ms
