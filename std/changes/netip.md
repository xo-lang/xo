# std/netip changes

## 2026-10-21: behavior: the Go backend runs netip.xo too

- Kind: behavior
- Commit: (filled in after the push)
- Found by: decision 0312 (std written once)
- Go reference: net/netip (go1.26.1), as ported in std/native/netip.xo
- Test: internal/stdtest/testdata/corpus/netip (go, arm64, LLVM); golden http_client_ip (Go parity)

The Go backend compiles std/native/netip.xo instead of wrapping Go's
net/netip; the display methods and the runtime's client_ip conversion stay in
std/rt/netip.go. Results are the same. On the Go backend 200,000 loops of two
parses, is_private, and two displays take 99 ms (Go's package 59 ms, the old
wrapper 49 ms): 1.7 times Go's package.

## 2026-10-21: perf: addresses written without per digit strings

- Kind: perf
- Commit: (filled in after the push)
- Found by: XO_PROF=cpu of a parse and display loop (string slices and concatenations per hex digit)
- Go reference: net/netip (go1.26.1)
- Test: internal/stdtest/testdata/corpus/netip (go, arm64, LLVM)

An IPv6 address is written into one byte builder (no substring or
concatenation per hex digit), its parse keeps one zeroed word array and reads
each digit once. 200,000 loops of two parses, is_private and two displays,
median of 5: native 126 ms to 72 ms (Go's package 37 ms, which includes its
Sprintf of the inputs). Results are the same.

## 2026-10-20: port: std/netip, Go's net/netip addresses and prefixes

- Kind: port
- Commit: d7409fb1
- Found by: survey
- Go reference: net/netip/netip.go (go1.26.1)
- Test: std/native/netip_test.xo (Go's results on 3,323 address and prefix cases)

New module: `parse_addr`, `parse_prefix`, `addr_from_bytes`, the Is
predicates, `unmap`, `bytes`, `display`, and Prefix `contains`, `masked`,
`display`, with Go's results and error texts on every backend.
