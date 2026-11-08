# 0561: BytesBuilder.write_le and write_be append a word's low bytes

Status: Accepted (owner, 2026-11-07). Drafted by Xo Lang 2 at the lead's request.

## Context

decision 0399 gave BytesBuilder `put_le` and `put_be`, which overwrite n
(1, 2, 4, 8) bytes already written. Appending a fixed width integer has
no primitive: a writer either appends a byte at a time (cbor's put_be
loop, four or eight write_byte calls each with its own capacity check)
or makes room first by writing zeros and then overwrites (netip's
string4: `write(ZERO16)`, two `put_le`, a slice). netip/display4 is
1.41x Go's (bench/results/xo-vs-go-survey-2026-11-07.md); the digits are
already packed into two words, and what remains is that builder dance.
The native runtime's string pushes (rt.rs_push_word) do it in one store,
but netip is compiled for the Go backend too (decision 0312), where rt
has no such form.

## Decision

Two prelude methods, as encoding/binary's AppendUint16/32/64 do for
byte slices:

    fn (var b: BytesBuilder) write_le(v: Int, n: Int) // append v's n low bytes, lowest first
    fn (var b: BytesBuilder) write_be(v: Int, n: Int) // the same, highest first

n is 0 to 8 (any count, unlike put_le's 1, 2, 4, 8: string4 writes 7 to
15 bytes as 8 plus the rest); a fault outside it. Native: one reserve
and one unaligned 8 byte store, the length growing by n (the bytes past
n are inside the capacity and not part of the value). Go backend
(std/rt list.go): `append` of binary.LittleEndian.AppendUint64's first
n bytes. wasm: the same as native in rt2.xo (gotchas: rt intrinsic
needs wasm) unless it is written in Xo over existing primitives.

## Consequences

- netip string4: with_capacity(16), write_le(lo, min(n, 8)), write_le(hi,
  n - 8) when n > 8, freeze, cast: no zero fill, no slice. Measured with
  --ab display4 n=7 on host 1 before landing.
- cbor put_be and put_head, protobuf fixed32/64, binary writers: one call
  where they loop.
- spec/std.md BytesBuilder list and the decision 0399 paragraph;
  std/changes/prelude.md; TestStdIndexCurrent, stdgen.

## Alternatives

1. Go forms for rt.rs_push_word(s): rt.* in a go_compiled module beyond
   rt.cast becomes a new rule, and it serves only Str building.
2. A netip-only Str constructor from two words: one caller.
3. Keep the builder dance: display4 stays at 1.41x.

## Landed (2026-11-07)

- Prelude `BytesBuilder.write_le(v, n)` and `write_be(v, n)` on every
  backend: native (llrt rs_write, one capacity check and one store),
  LLVM, wasm (`$rs_write`) and Go (std/rt M_write_le, M_write_be); a
  width outside 0 to 8 faults ("BytesBuilder.write_le and write_be:
  width must be 0 to 8"). Golden bytesbuilder_write (widths 0 to 8 in
  both orders, a negative value, growth across the capacity), checked on
  arm64, LLVM and Go parity.
- netip's string4 appends with write_le; ZERO16 and its overwrite are
  gone.
- Measured: bench/perf/netip on linux/arm64 host 1, --ab against main,
  7 rounds: display4 1.006 (0.963 to 1.079), parse4 0.946 (0.921 to
  1.008), both within noise. The write primitive does not move display4
  measurably (its 9 ms run is noisy); where the rest of the gap to Go
  (1.41x in the survey) goes is not yet profiled. No speedup is claimed;
  the primitive stays for fixed width writers.
