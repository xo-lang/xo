# Byte records against Go's bytes.Buffer, 2026-11-03 (decision 0480)

Linux/arm64 offload host (the first, exclusive). bench/perf/bytebuf at
ae5e9d89c against its Go twin, which cuts every workload with a
bytes.Buffer (Write, IndexByte over the unsearched bytes, Next): `xo bench
--ab` with Go as A and Xo as B, 15 rounds (twin-bb.txt; script twin.sh).
Records arrive in 4 KiB pieces; `_concat` is today's `rest + piece`,
`_views` is decision 0480's option B3 (a record inside a piece is a view
of it; one that spans pieces keeps views of them and is joined once).

| workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | Go peak RSS (KB) | Xo peak RSS (KB) | RSS Xo / Go |
|---|---|---|---|---|---|---|---|
| small_concat (1,006,704 lines of 20 to 80 bytes) | 14.52 | 13.34 | 0.919 | 0.905-0.938 | 12072 | 2196 | 0.182 |
| small_views | 14.66 | 13.43 | 0.916 | 0.905-0.941 | 12072 | 2196 | 0.182 |
| large1_concat (one 1 MiB line) | 1.049 | 1.028 | 0.98 | 0.802-1.015 (noise) | 9852 | 3816 | 0.387 |
| large1_views | 1.268 | 0.99 | 0.78 | 0.372-1.078 (noise) | 9844 | 1684 | 0.171 |
| large64_views (one 64 MiB line) | 26.97 | 5.42 | 0.201 | 0.177-0.26 | 270636 | 67372 | 0.249 |

large64_concat is not run: `rest + piece` copies the unread rest for each
of 16,384 pieces (about 512 GB of copying), the quadratic case the ADR
describes. The owner's bar for B1 (peak RSS no worse than Go's on the 1M
small records and the 1 MiB and 64 MiB records) is met by B3 with no new
type: 0.182, 0.171 and 0.249 of Go's peak, and the wall at or below Go's.
Per the owner's condition, the choice goes back to the owner before B1
is built. The held-views case (plan item 3) and Go's buffer_test port
(item 4) belong to B1 and are not run.

## B1, the prelude ByteBuffer (decision 0480 as accepted, 2026-11-03)

The same host and twin, `_buf` (B1: `b.write(piece)` then `while let
Some(rec) = b.read_until(0x0A)`), at each step of the implementation;
the B3 against B1 rows are `xo bench --ab` with B3's program as A and
B1's as B (bbab.sh, bbab2.sh).

| form | small_buf / Go | B1 / B3 small | large64 / Go | peak RSS / Go (small, large1, large64) | file |
|---|---|---|---|---|---|
| v1: one storage Str, Go's grow | 0.993 | (noisy) | 0.277 | 0.182, 0.398, 0.272 | twin-bb1.txt |
| v2: the written pieces kept as views | 1.175 | 1.212 | 0.215 | 0.183, 0.172, 0.249 | twin-bb2.txt |
| v3: read_until fast path for the head piece | 1.027 | 1.305 (spread 35%) | 0.198 | 0.183, 0.172, 0.249 | twin-bb3.txt |
| v4: the fast path without faults, inlined | 1.019 | 1.074 | 0.208 | 0.183, 0.172, 0.249 | twin-bb4.txt |
| (dropped) v5: head piece in a field of its own | 1.151 | 1.231 | 0.227 | same | twin-bb5.txt; v4 against v5 1.132 worse (ab-bbv45.txt) |

v5 ran 5% fewer instructions than v4 (callgrind in Docker, linux/arm64,
270.9M against 285.3M for the whole program) and was 13% slower on the
host; the cause is not known, and v4 ships.

B3's `_views` counts a record inside a piece by its index and never makes
it; `_vrec` makes each as a view, as B1's read_until and Go's Next return
it (twin-bb6.txt, v4):

| workload | Xo / Go wall | interval | B1 / B3 |
|---|---|---|---|
| small_buf | 1.007 | 0.992-1.041 | |
| small_views | 0.913 | 0.896-0.927 | 1.074 (twin-bb4.txt) |
| small_vrec | 0.992 | 0.764-1.132 | 1.049 (1.031-1.101) |

The owner accepted B1 on 2026-11-03 judged against `_vrec` (the same
work): 1.049 on 50 byte records, equal on 1 MiB and 64 MiB records, and
the same peak RSS as B3 at every size (0.183, 0.172 and 0.249 of Go's).
The cost of an Option[Bytes] return in the inlined loop went to the
compiler queue.
