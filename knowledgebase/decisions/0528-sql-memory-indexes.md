# 0528: std/sql memory driver: indexes on UNIQUE columns and writes in place

Status: Proposed (Xo Lang 2, 2026-11-05). Stub; the design follows.

## Context

Found by decision 0520's load check (bench/results/stability-load-2026-11-05.md). At 4000/s, stability/notes' p50
was 1.47 times its Go twin's. A variant with the twin's map store cut xo's CPU per request from 83.5 to 28 us.
Every statement of the memory driver scans its table, and every write copies the table under the Db's lock.
