kind: gotests
module: httpproxy
result: pass
commit: 518f3d020
source: 802274179827d15ca7667472fca277dccdc91c6cb56310b63786f506778def5d
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" httpproxy
measure: 54 cases from 2 tables of Go's tests (go1.26.11, golang.org/x/net v0.62.0), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/httpproxy/gotests/go) and the module (std/evidence/httpproxy/gotests/main.xo), every result equal, error text included.

- golang.org/x/net/http/httpproxy proxy_test.go UseProxyTests (use): 27 cases, 0 skipped
- golang.org/x/net/http/httpproxy proxy_test.go UseProxyTests (useall): 27 cases, 0 skipped

- arm64: 54 cases, 0 mismatches, 5ms
- llvm: 54 cases, 0 mismatches, 4ms
