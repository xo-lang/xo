kind: gotests
module: idna
result: pass
commit: 518f3d020
source: 43b4baadf7fdc861b507d754c17e8964dbf7cf90ebef483c2d75810202155e30
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" idna
measure: 608 cases from 32 tables of Go's tests (go1.26.11, golang.org/x/net v0.62.0), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/idna/gotests/go) and the module (std/evidence/idna/gotests/main.xo), every result equal, error text included.

- golang.org/x/net/idna idna_test.go idnaTestCases (alookup): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (ulookup): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (aregistration): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (uregistration): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (adisplay): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (udisplay): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (apunycode): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (upunycode): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (alookup): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (ulookup): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (aregistration): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (uregistration): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (adisplay): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (udisplay): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (apunycode): 7 cases, 0 skipped
- golang.org/x/net/idna idna_test.go idnaTestCases (upunycode): 7 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (alookup): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (ulookup): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (aregistration): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (uregistration): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (adisplay): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (udisplay): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (apunycode): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (upunycode): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (alookup): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (ulookup): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (aregistration): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (uregistration): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (adisplay): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (udisplay): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (apunycode): 31 cases, 0 skipped
- golang.org/x/net/idna punycode_test.go punycodeTestCases (upunycode): 31 cases, 0 skipped

- arm64: 608 cases, 0 mismatches, 4ms
- llvm: 608 cases, 0 mismatches, 6ms
