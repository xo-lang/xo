# 0485: netip.Addr as a 128-bit value, like Go's (proposal)

Status: Proposed (Xo Lang 3, 2026-11-03; asked for by the lead). The owner decides before any code.

## Context

`netip.Addr` is `struct { bits: Int, ip: Bytes, zone: Str } derive Ord` (std/netip.xo, decision 0287): the 16
address bytes live in a heap `Bytes`. Go's `netip.Addr` is `{addr uint128, z *intern.Value}`: a value of 16 bytes
plus a pointer, no allocation per address.

bench/perf/netip (one parse of two addresses, `is_private`, and the display of both, 200,000 times), host 1,
bench/results/std-twin-netip-2026-11-03.md:

| step | Xo / Go (wall) |
|---|---|
| before this session | 1.88 |
| string4/string6 without a UTF-8 check, byte scan for `%` (bce92494e) | 1.79 |

The Mac profile of the same program (4,000,000 iterations) puts `parse_ipv6` at 31 percent cumulative, `parse_addr` at 55,
`ipv4_fields` at 12, `drop Str` at 11, `string6` at 9 and `cabi.obj_rs_push_byte` at 8: the parsers build a `BytesBuilder`
and freeze it into the `Addr`'s `ip` for every address, the displays build a `BytesBuilder` for the text, and each `Addr`
is freed with its `Bytes`. Two of these are the representation: the `Bytes` allocation (and its release) and the
byte-at-a-time construction of the 16 bytes.

## Decision (proposed)

Replace `ip: Bytes` with two `Int` words:

```
struct Addr {
  bits: Int        // 32, 128 or 0, as now
  hi: Int          // the first 8 bytes, big endian (an IPv4 address in ::ffff:a.b.c.d form: hi 0, lo 0xffff00000000 | v4)
  lo: Int          // the last 8 bytes
  zone: Str
}
```

- `Ord` becomes a written `compare` (Go's `Addr.Compare`: bit length, then `hi` then `lo` as unsigned, then zone): a
  `derive Ord` on signed words would put addresses with the top bit set first.
- `bytes()`, `addr_from_bytes`, `display` and the predicates read and write the words with shifts; `is_private`,
  `is_loopback` and the others become comparisons of `hi` and `lo`, as Go's.
- Parsers accumulate the words as they read groups and digits, with no builder. `string4` and `string6` keep one
  `BytesBuilder` per call (the text is the result).
- The field `ip` was public. A program that read `addr.ip` must use `addr.bytes()` (which returns 4 or 16 bytes, Go's
  `AsSlice`) or `addr_from_bytes`. Go's fields are private; the spec lists `ip` only as the implementation (spec/std.md
  28), so the change is a break for readers of that one field.

## Consequences

- No allocation per `Addr` for the bytes; copies are three words and a `Str` (the empty zone is a static).
- A break for programs that read `.ip` (to be searched in bench/paper, examples and std: expected none); a CHANGELOG
  Breaking entry and a migration line in spec/std.md 28.
- Equality and hashing derive from the fields as now.
- The Go backend (std/rt) and the native module (std/native/netip.xo) both change; the corpus (Go-generated) stays.

## Measurement plan

1. Host 1, `xo bench --ab --early` on bench/perf/netip against the parent: expect the 1.79 to fall below 1.3 from the
   allocation alone; Proven is at most about 1.05 of Go (decision 0461).
2. Add workloads to the twin: `parse4`, `parse6`, `display6`, `compare` (a sort of 10,000 addresses), so the remaining gap
   is attributed (the twin today mixes them).
3. Instruction counts (callgrind on the host) for the parse and display loops, to separate the representation from the
   parser's own cost.

## Alternatives

- **Keep `ip: Bytes`, reuse one per call**: nothing to reuse across addresses; the `Addr` owns its bytes.
- **A fixed size array field** (`ip: [UInt8; 16]`): inline storage without two words; Xo has no such field type in
  structs yet (core 2.x), and word compares are what the predicates want.
- **Do nothing**: the program stays at 1.79 of Go's wall; the cost is the allocation and the byte loops, not an
  algorithmic gap.
