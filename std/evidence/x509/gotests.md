kind: gotests
module: x509
result: pass
commit: a28d92ce0
source: 6c03a98a635cc87dc8b35d309ec8245fc1382518b3bceb4394a2a34292086671
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" x509
measure: 37 cases from 3 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 9 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/x509/gotests/go) and the module (std/evidence/x509/gotests/main.xo), every result equal, error text included.

- crypto/x509 oid_test.go oidTests (oid): 16 cases, 9 skipped
- crypto/x509 oid_test.go TestInvalidOID.cases (inv): 6 cases, 0 skipped
- crypto/x509 oid_test.go TestOIDMarshal.cases (parse): 15 cases, 0 skipped

- arm64: 37 cases, 0 mismatches, 8ms
- llvm: 37 cases, 0 mismatches, 4ms
