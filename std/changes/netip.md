# std/netip changes

## 2026-11-04: perf: display of an IPv4 address packs its octets in two words; IPv6 hex digits come from a table

- Kind: perf
- Commit: this push
- Found by: the host 1 rows after decision 0485 (std-twin-netip-2026-11-03.md): display4 1.98, parse6 1.70 of Go
- Go reference: net/netip Addr.string4 (appendDecimal into a stack array), parseIPv6's hex loop
- Test: the netip corpus (arm64, LLVM, Go backend) and `xo test --std-module=netip`
- Timing: host 1 A/B display4 0.637 of before (0.629-0.645), parse6 0.901 (0.884-0.925), both n=6
- Shipped in: not yet released

`string4` builds the text in two registers (each octet's digits and dot by shifts and ors), stores them with two `put_le` into a 16 byte builder and cuts the result to its length, in place of up to 15 `write_byte` calls. `parse_ipv6` reads a hex digit with one load from a 256 byte table. An in-place `ipv4_fields` (no slice) was tried and measured worse (parse4 1.03, n=15), so it stays as it was.

## 2026-11-03: perf, breaking: Addr holds its 16 bytes as two words

- Kind: perf (breaking)
- Commit: this commit
- Found by: bench/perf/netip 1.79 of Go's wall (decision 0485)
- Go reference: net/netip (go1.26.1): `Addr{addr uint128, z}`
- Test: the netip, x509 and http corpora (arm64, LLVM, Go backend), twincheck bench/perf/netip
- Timing: host row follows in std-twin-netip-2026-11-03.md

`Addr` is `{bits, hi, lo, zone}` with the top bit of each word flipped so that the derived order of the signed words is the order of the bytes (as `Uuid`). The parsers build the words with shifts (a `::` moves the later groups with a 128-bit shift), the predicates and `Prefix.contains` compare masked words, and `string4`/`string6` read octets and groups from them. Breaking: the field `ip: Bytes` is gone; read `a.bytes()` and build with `netip.addr_from_bytes(b)`. std/x509's IP SANs and the corpus helpers use `bytes()`.

## 2026-11-03: perf: address text is returned without a UTF-8 check, and parse_ipv6 scans for % by byte

- Kind: perf
- Commit: this commit
- Found by: bench/perf/netip 1.88 of Go's wall on host 1 (n1: Go 19.0 ms, Xo 35.8); Mac profile: parse_ipv6, parse_addr, ipv4_fields, `drop Str`
- Go reference: net/netip (go1.26.1)
- Test: the netip corpus (arm64, LLVM, Go backend), twincheck bench/perf/netip
- Timing: host row follows in std-twin-netip-2026-11-03.md

`string4` and `string6` return their built bytes as the Str they are (digits, dots, colons, hex and the zone Str) without `to_str`'s validation, and `parse_ipv6` finds the zone separator with a byte loop instead of `Str.find`.

## 2026-10-29: feature: netip.lookup

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md S7)
- Go reference: net.LookupIP; decision 0406
- Test: internal/llvm/testdata/golden/net_lookup_dial_timeout

`netip.lookup(net, host)` returns host's addresses as `netip.Addr`, through
`Net.lookup_host`.

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
