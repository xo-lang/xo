# Memory growth runs that share a host inflate each other's growth

Status: Observed (2026-11-19, task #355, xo-4c).

Symptom: two grpcreflect 60 minute stress runs (`stdproven stress
-minutes 60`), started together on linux/arm64 host 2 (18 CPUs), both
failed the growth check: 34% and 25%, each with about 180 million
rounds. Run alone on the same host, one after the other, the same two
trees grew 3% and 3%, and 3% and 2%, with about 660 to 680 million
rounds each. Both runs had also hit their lease limit (rc 124) before
the second try.

Cause: each stress program runs 64 tasks. Two of them on 18 cores
starve each other, so the runs do a third of the work and the
resident set's mark (minutes 30 to 32) and end (the last 3 minutes)
sample a heap shaped by scheduling, not by the allocator. The pair
compares the two runs' contention, not the two trees.

Fix: run growth comparisons one at a time with an exclusive lease
(`SP_EXCLUSIVE=1 OFFLOAD_ENV=offload-arm64b.env stdproven-offload.sh
...`), both sides of an A/B on the same host, and give the lease room
for stdproven's second try (OFF_MAX_MIN about 140 for a 60 minute
run). A pair split across hosts cannot be compared either.
