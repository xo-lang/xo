kind: gotests
module: norm
result: pass
commit: 518f3d020
source: 2bcf53a513f181d80ee492f3bbda04e23d9091a43a8ee5414aca65937bff9f63
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" norm
measure: 308 cases from 28 tables of Go's tests (go1.26.11, golang.org/x/text v0.43.0), 2 backends (arm64,llvm), 0 mismatches with Go, 272 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/norm/gotests/go) and the module (std/evidence/norm/gotests/main.xo), every result equal, error text included.

- golang.org/x/text/unicode/norm normalize_test.go isNormalTests (isnfc): 8 cases, 1 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalTests (isnfd): 8 cases, 1 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalTests (isnfkc): 8 cases, 1 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalTests (isnfkd): 8 cases, 1 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalNFDTests (isnfc): 12 cases, 0 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalNFDTests (isnfd): 12 cases, 0 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalNFDTests (isnfkc): 12 cases, 0 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalNFDTests (isnfkd): 12 cases, 0 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalNFCTests (isnfc): 14 cases, 0 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalNFCTests (isnfd): 14 cases, 0 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalNFCTests (isnfkc): 14 cases, 0 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalNFCTests (isnfkd): 14 cases, 0 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalNFKXTests (isnfc): 1 cases, 0 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalNFKXTests (isnfd): 1 cases, 0 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalNFKXTests (isnfkc): 1 cases, 0 skipped
- golang.org/x/text/unicode/norm normalize_test.go isNormalNFKXTests (isnfkd): 1 cases, 0 skipped
- golang.org/x/text/unicode/norm normalize_test.go appendTestsNFC (nfc): 11 cases, 8 skipped
- golang.org/x/text/unicode/norm normalize_test.go appendTestsNFC (nfd): 11 cases, 8 skipped
- golang.org/x/text/unicode/norm normalize_test.go appendTestsNFC (nfkc): 11 cases, 8 skipped
- golang.org/x/text/unicode/norm normalize_test.go appendTestsNFC (nfkd): 11 cases, 8 skipped
- golang.org/x/text/unicode/norm normalize_test.go appendTestsNFKC (nfc): 30 cases, 46 skipped
- golang.org/x/text/unicode/norm normalize_test.go appendTestsNFKC (nfd): 30 cases, 46 skipped
- golang.org/x/text/unicode/norm normalize_test.go appendTestsNFKC (nfkc): 30 cases, 46 skipped
- golang.org/x/text/unicode/norm normalize_test.go appendTestsNFKC (nfkd): 30 cases, 46 skipped
- golang.org/x/text/unicode/norm normalize_test.go appendTestsNFKD (nfc): 1 cases, 13 skipped
- golang.org/x/text/unicode/norm normalize_test.go appendTestsNFKD (nfd): 1 cases, 13 skipped
- golang.org/x/text/unicode/norm normalize_test.go appendTestsNFKD (nfkc): 1 cases, 13 skipped
- golang.org/x/text/unicode/norm normalize_test.go appendTestsNFKD (nfkd): 1 cases, 13 skipped

- arm64: 308 cases, 0 mismatches, 5ms
- llvm: 308 cases, 0 mismatches, 5ms
