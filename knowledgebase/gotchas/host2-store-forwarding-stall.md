# A wide load over fresh narrow stores stalls on linux/arm64 host 2, not on the Mac

Status: Observed (2026-11-20, task #429, xo-4c)

Symptom: the same binary's loop runs 2.7x slower on linux/arm64 host 2 and at the same speed on
the Mac. bench/perf/gobridge's Xo twin with `s.bytes().clone()` (f6d29ea74) took 70.3 ms for the
bytes row on host 2 against 26.3 ms for `s.bytes()`; on the Mac 35.9 vs 37.2 ms. The runs were
alternated, with 0 context switches and 0 migrations (perf stat), so it was not host load.

Cause: Go inlined the clone into a 32-byte stack buffer at 160(RSP), 16 mod 32. The loop
stored two 8-byte spills at 144(RSP) and 152(RSP) each iteration. Go's
internal/bytealg.IndexByteString rounds the pointer down to 32 (`AND $-32, R0, R3`) and loads 32
bytes (`VLD1.P 32(R3)`), so the load covered both fresh stores. A load wider than the stores it
overlaps cannot be forwarded and waits for them to drain, about 13 cycles a call on host 2's
cores; Apple cores do not show the cost. The same clone into a heap block ran 25.8 ms.

Fix: find it by hot IPs on the host (perf-annotate-empty-on-host2.md) and look for a vector or
pair load whose block covers a store of the same iteration. Keep hot buffers off the spill
area (a heap block, or no copy at all), and never judge a layout change from the Mac alone:
measure on host 2 (ADR 0712). Notes: ~/xo-lead/notes/xo-4c/429-bytes-gap-2026-11-20.md.
