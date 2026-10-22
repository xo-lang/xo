# Mapped bytes overstate the heap's memory

Status: Observed (2026-10-22, decision 0349's first memory budget)

Symptom: with decision 0349's first default (half of physical memory,
12 GB on the 24 GB Mac), every build of xo by an xo of ed1b10c83 stopped
with `fatal error: memory budget exceeded: 12289 MB mapped ... past phase
fn_keys`, while the same build's resident set stayed under heavy.sh's
12 GB. Every agent's xobuild failed until 75296384b removed the default.

Cause: the budget counted the bytes heapos.xo maps. A growing large
block (a list's or a string's doubling, large_resize) maps twice its new
size so the next growth stays in place, and those pages are reserved,
not touched; the large block cache keeps up to 64 MiB more. Mapped bytes
ran far past what the process used.

Fix: the budget counts large blocks by the bytes handed out (large_room
adds n, an in-place resize the difference, release subtracts it) and
chunks and records as mapped. A cold self build of xo: peak 9,909 MB
counted against 9,141 MiB of maximum resident set (bench/results/memory-budget-measure-2026-10-22.md).

How to avoid: before a default limit, measure the limit's own counter
against RSS on the heaviest real workload (a self build, the full
tier), not the workload's RSS alone.
