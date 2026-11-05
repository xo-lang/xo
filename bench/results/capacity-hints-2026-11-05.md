# Capacity hints: which reach the allocation (2026-11-05)

An audit after 1b4b2bfa1 (`Buf.with_capacity` evaluated its size and dropped
it on every native build). Every size or capacity hint in the prelude and
std, per backend, before and after this work:

| hint | native arm64/amd64 | LLVM | wasm | Go backend |
|---|---|---|---|---|
| `List.with_capacity(n)` | honored (0170) | honored | honored (`$rl_new_n`) | honored |
| `Buf.with_capacity(n)` | dropped, honored since 1b4b2bfa1 | same | same (SSA `list.new` with a size) | honored |
| `BytesBuilder.with_capacity(n)` | honored (`rs_with_cap`) | honored | honored (`$rs_with_cap`) | honored |
| `ByteBuffer.with_capacity(n)` | not used, by measurement (below) | same | same | honored |
| `deque.new(cap)` | length honored, ring built by pushing onto `[]` (about log2(cap) copies); now sized once | same | same | honored |
| deque growth | new ring built from `[]`; now sized once | same | same | Go append |
| `Str.repeat`, `Bytes.repeat` | presized | same | same | Go |
| `bufio.reader(s, size)` | honored (each read asks for size) | same | same | honored |
| `Chan.new(cap)`, `trace.ring(cap)` | sizes, not hints (semantics) | | | |

Map, Set, and StrBuilder have no hint form.

Native, LLVM and wasm share the lowering (compiler/ssa) and std/native,
so one fix covers all three.

## Measurements

`xo bench --ab`, 15 rounds, A = std of main, B = the change, the same xo
(hintbench: 20 rounds of 200,000 push_back; `deque` from new(0),
`dequecap` from new(200_000); `bb` 125,000 writes of 32 bytes into
`ByteBuffer.with_capacity(4_000_000)`):

| program | linux/arm64 (host 1) | darwin/arm64 (Mac) |
|---|---|---|
| deque | 0.614 (57 to 35 ms) | 0.844 (32 to 27) |
| dequecap | 0.741 (27 to 20) | 0.765 (17 to 13) |
| bb, tail sized to n | 1.882 worse (17 to 32) | 1.0 (23 to 23) |

The ByteBuffer change was dropped: with writes kept as views, a large first
tail only costs (one 4 MB block per buffer against the tail's own growth).

## Known-size lists built from [] (the same cost without a hint)

The audit's search (a `var x = []` followed by a `for` over a known count
that pushes onto it) found 75 sites in std/native; most are 8 to 80
elements or cold. The prelude's run in user code and now size their
result once from 16 elements (PRESIZE_MIN; shorter ones grow from `[]` as
before): List.map when U is not T (decision 0508's in-place path covers
U = T), List.indexed, Bytes.indexed, List.group_by's result, and
List.sort_by's key and index lists.

`xo bench --ab`, A = std of main, B = the change (hintbench `mapconv`:
1M Ints mapped to pairs, 10 times; `indexed`: 1M Ints indexed, 10 times):

| program | linux/arm64 (host 1) | darwin/arm64 (Mac) |
|---|---|---|
| mapconv | 0.85 (60 to 51 ms) | 0.80 (40 to 32) |
| indexed | 0.862 (58 to 50) | 0.769 (39 to 30) |

Sized at every length (no threshold), bench/perf/listmap on the linux/arm64
ab-host read `small` 1.04 and `sortby_small` 1.039 worse (2 to 4 element
lists: rl_new_n costs more than the first push's growth), `sortby` 0.839;
with the threshold of 16 the short path still read `small` 1.034 (31
rounds; list_map's larger body), so the sized map moved out of line
(list_map_sized): `small` 1.019 (within noise, 31 rounds), `sortby_small`
1.041 (within noise, 21), `mid` 0.779 (32 element lists, now sized),
`sortby` 0.738.

bench/perf/deque (linux/arm64 ab-host, 10 rounds): wall 0.999 (its ring
stays small).

