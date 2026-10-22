# std/netip changes

## 2026-10-22: perf: parse and display without per group strings and lists

- Kind: perf
- Commit: dd05b4c3a
- Found by: the std Go ranking (netip 1.85x Go's net/netip), XO_PROF=cpu
- Go reference: net/netip (go1.26.1)
- Test: internal/stdtest/testdata/corpus/netip (go, arm64, LLVM); golden http_client_ip (Go parity)

`parse_ipv6` walks the text by an index instead of slicing the rest at every
group, writes the bytes straight into a builder (the `::` gap is filled when
the address ends, not by moving a 16 word list), and the IPv4 and IPv6
displays write digits into one builder (`string6` also skips past a zero run it has counted, as
Go does). 200,000 loops of two parses, `is_private` and two displays: 1,016 M
to 872 M instructions (native arm64; `xo bench --ab` wall 0.829, instructions
0.858); Go's net/netip on the same loop (no Sprintf): 551 M, so 1.85x to 1.58x.
Results are the same. bench/results/netip-2026-10-22.md.

## 2026-10-21: behavior: the Go backend runs netip.xo too

- Kind: behavior
- Commit: 2f7d03c8
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
- Commit: 3a2d9f52
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
