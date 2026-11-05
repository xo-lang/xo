# Buf.with_capacity reserves its size (2026-11-05)

micro13 sort1M fills `Buf[Int].with_capacity(1_000_000)` and sorts it in
place. A perf profile on the linux/amd64 offload host (20 kHz, call chains
mapped through nm) put 58 of 1,616 samples in memcpy under obj.hresize in
main's fill loop: the SSA builder lowered `Buf.with_capacity(n)` to
`list.new` with no size, so the buffer grew from empty. The partition loops
were even with Go's (1,045 samples against slices.partitionOrdered's 1,091;
insertion sort 183 against 163).

`xo bench --ab`, A = main 149c5de3a (darwin: d51f566bf, the same compiler
for this), B = the fix (285f6811f, before its docs):

| platform | rounds | work_ms | wall | cpu |
|---|---|---|---|---|
| linux/amd64 (offload host) | 21 | 0.938 (65 to 61) | 0.932 | 0.932 |
| darwin/amd64 (Intel laptop) | 21 | 0.973 (75 to 73) | 0.985 | 0.985 |
| linux/arm64 (ab-host) | 15 (stopped at 6) | | 0.986 | 0.986 |

Against Go 1.26.1 (slices.Sort): before the fix linux/amd64 work 64 vs 65
(1.016 worse); with it 61. darwin/amd64 with the fix: work 75 vs 74 (0.987,
noise), wall 0.971.
