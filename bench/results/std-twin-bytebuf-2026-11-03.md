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
