kind: gotests
module: netip
result: pass
commit: aefc47b57
source: 1a59b19fc612328095d6f6eea1dae91dc644feea4557db49a426aa34e25a5f3b
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" netip
measure: 135 cases from 11 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 17 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/netip/gotests/go) and the module (std/evidence/netip/gotests/main.xo), every result equal, error text included.

- net/netip fuzz_test.go corpus (corpus): 58 cases, 0 skipped
- net/netip netip_pkg_test.go nextPrevTests (props): 10 cases, 0 skipped
- net/netip netip_pkg_test.go nextPrevTests (props): 8 cases, 2 skipped
- net/netip netip_pkg_test.go TestPrefixContains.tests (contains): 15 cases, 9 skipped
- net/netip netip_pkg_test.go TestParseIPError.tests (addr): 5 cases, 0 skipped
- net/netip netip_pkg_test.go TestParseAddrPort.tests (addrport): 7 cases, 0 skipped
- net/netip netip_test.go TestParsePrefixError.tests (prefix): 12 cases, 0 skipped
- net/netip netip_test.go TestPrefixMasked.tests (prefix): 2 cases, 2 skipped
- net/netip netip_test.go TestPrefixMasked.tests (prefix): 2 cases, 2 skipped
- net/netip netip_test.go TestIs4AndIs6.tests (props): 6 cases, 1 skipped
- net/netip netip_test.go TestIs4In6.tests (props): 10 cases, 1 skipped

- arm64: 135 cases, 0 mismatches, 7ms
- llvm: 135 cases, 0 mismatches, 5ms
