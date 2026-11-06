# std/sql memory driver: CREATE INDEX (2026-11-06)

Host 1 (linux/arm64), revision c5a0beb3b (the CREATE INDEX change, before
this file), `bench/perf/sqlmem owner|owner_idx 20000`: a notes table of
20,000 rows over 100 owners, then 2,000 statements of `SELECT * FROM notes
WHERE owner = $1 ORDER BY id` (200 rows each). `owner` scans; `owner_idx`
declares `CREATE INDEX notes_owner ON notes (owner)` first. One build, the
two workloads alternated, 5 runs each, wall time of the whole program
(table build included). Script: Xo Lang 2's sqltime.sh (offload host 1).

| workload | runs (ms) | median (ms) |
|---|---|---|
| owner (scan) | 4358, 5274, 4278, 4250, 4847 | 4358 |
| owner_idx (index) | 264, 316, 272, 275, 275 | 275 |

The index is 15.8x faster on this load; both print the same checksum
(400000). An index on a value most rows hold is scanned instead (a value
more than a quarter of the live rows hold), so stability/notes, whose
harness load has one owner, declares none.

The insert path with the change, against its parent f04a8b4ca
(`xo bench --ab`, host 1, n=5): `none 200000` wall 1.000 (0.985 to 1.011).
