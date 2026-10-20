# std/netip changes

## 2026-10-20: port: std/netip, Go's net/netip addresses and prefixes

- Kind: port
- Commit: (filled in after the push)
- Found by: survey
- Go reference: net/netip/netip.go (go1.26.1)
- Test: std/native/netip_test.xo (Go's results on 3,323 address and prefix cases)

New module: `parse_addr`, `parse_prefix`, `addr_from_bytes`, the Is
predicates, `unmap`, `bytes`, `display`, and Prefix `contains`, `masked`,
`display`, with Go's results and error texts on every backend.
