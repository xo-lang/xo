# 0552: Map slots carry the key's hash bits

Status: Accepted (the owner, 2026-11-07, together with decision 0545;
Xo Lang 4). Landed from the 0545 stack (l4-slot45r3, c662cb058) on top
of decision 0545's code.

## Context

A Map in the native and wasm backends is lists (compiler/ssa map.xo):
keys, values and live flags in insertion order, and a table of slots,
open addressing with linear probing, each slot 0 (empty) or an entry's
index + 1. A table is built from the fifth entry on (SMALL_MAP) and
rebuilt at 3/4 full into one at least twice the live count.

Two costs follow from a slot holding only the index:

- every rebuild hashes every live key again (`xo.map.rebuildk[K]`,
  `xo.map.rebuild[T]`): a Map grown from empty to n entries hashes each
  key about twice, once on its put and about once more over the
  rebuilds;
- every probe of a lookup reads the entry's live flag and compares its
  key, the probes that land on other keys included: a miss compares
  every key of its cluster.

A 10 kHz profile of 20 warm whole builds of http_server_request on the
linux/arm64 host (main 686282584, 2026-11-07): `ints.str_hash` was 7.3%
of the CPU, 22% of that from `xo.map.rebuildk[Str]` (1.6% of the
build); `obj.str_eq` 2.1%, much of it the map probes. The compiler's
keys are 64 hex digit cache keys in Str sets of 3,000 to 20,000.

Decision 0545 (Proposed) hashes Str keys a word at a time, which makes
each hash cheaper; this decision removes hashes and compares. Both
touch map hashing, so the numbers below are on main and on top of the
0545 prototype.

## Decision

A slot holds the low 31 bits of its key's hash above the entry index:
`slot = (h & 0x7fffffff) << 32 | (pos + 1)`, 0 still empty.

- `xo.map.find` and `xo.mlog.lprobe` (a maplog lists level) compute
  `h & 0x7fffffff` once and, at each full slot, compare it with the
  slot's `slot >> 32` first; only an equal tag reads the live flag and
  compares the key. The entry index is `(slot & 0xffffffff) - 1`.
- `insert_slot` writes the word (every put, both rebuilds).
- A rebuild (`refill_slots`) with an old table walks the old slots and
  inserts each entry at its tag's home slot: no key is hashed. With no
  dead entry an entry keeps its index; with dead ones a list of the old
  count maps each old index to its index after the compaction (-1 for a
  dead entry). A small map's first table (no old slots) hashes its keys
  as before.
- Int keys hash again on a rebuild instead (their hash is a few
  instructions): the rebuild never reads the old table, so it dies
  before the new one is made, as before this decision. Reading it kept
  the old table alive beside the new one (round 2: Int maps' peak RSS
  1.09 to 1.14 times).
- 31 bits so the word is never negative and its shift never overflows
  (SSA's Shl is checked where the arm64 backend cannot prove it exact);
  a table of up to 2^31 slots finds a home slot from the tag alone,
  and an index fits the low 32 bits.

The maplog index (decision 0291's log, `idx`) is unchanged: its entries
already hold the full hash, which its probes compare before the key.
Iteration, removal and the lists' order are unchanged; only the table's
layout after a rebuild differs (entries are placed in old slot order,
not key order), which no program can observe.

## Results

linux/arm64 host 1, `ab-host.sh` (xo bench --ab, 3 rounds) of
bench/perf/mapops per mode, B/A of the median (wall and CPU agree to
0.001), and s1ab (20 warm whole builds a side, a back-end cache each);
2026-11-07. Round 1 is the first prototype (c38284738: find no longer
inlined outside loops, a rebuild always building its index list);
round 2 inlines find at every site (INLINE_FIND_INSTRS) and builds no
index list without dead entries; round 3, the final one, hashes Int
keys again on a rebuild.

Round 1, on main (c08d348d1 = main + the benchmark):

| mode | B/A | RSS B/A |
|---|---|---|
| insert_str (200k, from empty, 10x) | 0.634 | 0.988 |
| hit_str | 0.863 | 0.999 |
| miss_str | 0.758 | 0.999 |
| iter | 0.850 | 0.999 |
| delete | 0.732 | 0.988 |
| insert_int (1M, 5x) | 1.085 (worse) | 1.272 |
| hit_int | 0.958 (noise) | 1.275 |
| miss_int | 0.950 (noise) | 1.275 |
| s1ab json_decode_fields CPU | 0.978 | |
| s1ab http_server_request CPU | 0.978 | |

Round 1, on the 0545 prototype (37d6789fe):

| mode | B/A | RSS B/A |
|---|---|---|
| insert_str | 0.718 | 0.990 |
| hit_str | 0.825 | 1.000 |
| miss_str | 0.710 | 1.000 |
| iter | 0.916 | 1.000 |
| delete | 0.801 | 0.990 |
| insert_int | 1.072 (worse) | 1.275 |
| hit_int | 0.892 | 1.259 |
| miss_int | 0.984 (noise) | 1.317 |
| s1ab json_decode_fields CPU | 0.999 | |
| s1ab http_server_request CPU | 1.007 | |

The Int rows' RSS and insert_int came from the rebuild's index list (8
bytes an entry, alive with both tables); round 2 builds it only when
the map has dead entries.

Native .text (round 1, linux/arm64): xo 37,537,568 to 37,446,352 bytes
(-0.24%), mapops +504, http_server_request -280.

Round 2 (2552fccfb + 6ede2778f + 700a11dfe on 0545; 4ed9c8b64 on
main), wall B/A (CPU within 0.002 of it):

| mode | on main | RSS | on 0545 | RSS |
|---|---|---|---|---|
| insert_str | 0.616 | 0.987 | 0.654 | 0.987 |
| hit_str | 0.817 | 0.999 | 0.773 | 0.999 |
| miss_str | 0.669 | 0.998 | 0.668 | 0.998 |
| iter | 0.847 | 0.997 | 0.866 | 0.999 |
| delete | 0.701 | 0.988 | 0.751 | 0.988 |
| insert_int | 0.918 | 1.110 | 0.949 | 1.113 |
| hit_int | 0.797 | 1.111 | 0.838 | 1.143 |
| miss_int | 0.824 | 1.127 | 0.896 | 1.092 |
| s1ab json_decode_fields CPU | 0.965 | | 1.004 | |
| s1ab http_server_request CPU | 0.965 | | 1.006 | |

Every mode is faster on both bases; round 2 turned insert_int from
1.085 to 0.918. The Int maps still peak 9 to 14% higher in RSS, most
likely (not yet measured) because a rebuild keeps the old slots list alive while it fills the new one (it
reads the tags from it), where the old rebuild dropped it first; the
Str maps' keys hide it. On the compiler, C saves 3.5% CPU on main and
nothing on top of 0545 (0545 already removes most of the compiler's
rehashing and misses).

Round 3 (9098a023f on main, c662cb058 on 0545), the Int modes (the
Str code is round 2's), wall B/A against the same bases (CPU within
0.001):

| mode | on main | RSS | on 0545 | RSS |
|---|---|---|---|---|
| insert_int | 0.850 | 1.000 | 0.904 | 0.998 |
| hit_int | 0.787 | 1.030 | 0.866 | 0.966 |
| miss_int | 0.851 | 0.966 | 0.859 | 1.012 |
| s1ab, round 3 / round 2, json CPU | 0.993 | | | |
| s1ab, round 3 / round 2, http CPU | 0.996 | | | |

The Int RSS is back to the base (within the 3% run spread), which
confirms the cause, and inserts are faster than round 2's (rehashing
an Int beats reading the old table).

Gate, round 3 against round 2 on host 2: self build, codegen 36/0, xo
test 115/0, internal/arm64 tests pass, identity 20 of 313 differing
(Int key maps), no new failure. Wasm (round 2, host 1):
TestWasmCommandGoldens and the arm64 goldens pass. LLVM (round 1
d5e046d28 with the find rule): every TestGolden passes.

Gate, round 2 against c08d348d1 on host 2: self build, codegen 36/0,
xo test 115/0, internal/arm64 tests pass, identity 313 compared and
110 differing (Map code changes output), no new failure.

## Consequences

Every Xo program's Map changes, not only the compiler's: the tag costs
one AND and one compare per full probe and saves a live flag load and a
key compare per probe on another key. Measured: every mapops mode is
faster on main (Str 0.62 to 0.85, Int 0.79 to 0.85) and on 0545 (Str
0.65 to 0.87, Int 0.86 to 0.90), RSS within the run spread of the base.
The compiler builds 3.5% faster (s1ab CPU) on main and the same on top
of 0545, which already removes most of its rehashing and misses: on its
own merits C is a user program change, and both orders work.

A rebuild of a Str (non Int) map with dead entries allocates one Int
list of the old entry count, and keeps the old table until the new one
is filled; a Str map's keys dwarf both (RSS 0.987 to 0.999). The arm64
inliner's find limit is 52 instructions (INLINE_FIND_INSTRS, was the
general 40) so find stays inlined with the tag check; native .text
moves under 0.3% (xo -0.24%, round 1). Map layout after a rebuild
differs, so builds are not byte identical to before.

## Alternatives

- Store the full 64-bit hash in a parallel list (as Go's tophash bytes,
  or Swiss tables' control bytes): a second list per Map, another load
  per probe and more memory; the tag fits in the slot word for free.
- Presize Maps (`Map.with_capacity(n)`): removes rebuilds only where the
  size is known up front, needs a std API and a change at each site,
  and does nothing for misses.
- Keep the slot and only cache hashes per key (a hash list beside keys):
  removes the rehash but not the compares, and costs 8 bytes an entry.
