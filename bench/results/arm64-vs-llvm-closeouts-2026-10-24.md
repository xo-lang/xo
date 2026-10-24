# arm64 against LLVM: four sweep gaps re-measured, 2026-10-24

Xo Lang 1. The sweep's arm64 gaps (portable arith32, trim, dur_parse,
fault defer) and RSA signing, re-measured on main with `xo bench --ab` in
the lead's quiet windows (macOS, M-series, instructions retired and
wall), every binary built with XO_INLINE_STRICT=1. A = LLVM, B = the
native arm64 backend, one xo for both sides; ratios are B / A.

| workload | rounds | wall | instructions | outcome |
|---|---|---|---|---|
| fault defer (bench/perf/fault) | 30 | 0.915 (0.907 to 0.924) | 1.056 | closed: native is faster on the wall; the instruction gap is in the unwinder (unwind.fc_restore, uleb, step and read_val take the same shares on both backends in XO_PROF) |
| RSA sign2048 (bench/perf/rsa) | 20 | 1.079 (1.03 to 1.088) | 1.155 | open: mac_row is 9 instructions a word with adds/adcs on arm64; both backends spend about 80% in mac_row and its stores |
| RSA sign4096 | 20 | 1.062 (1.057 to 1.068) | 1.152 | open, as sign2048 |
| dur_parse, a 2,000,000 parse loop | 30 | 1.238 before, 1.105 after e50722a01 | 1.056 before, 0.923 after | improved: each digit's byte read once, guarded overflow checks wrapping, the white space trim's calls only when an end byte needs them |
| trim (bench/perf/text) | 30 | 1.048 | 1.10 | open, smaller than dur_parse's |

The defer and RSA rows are from the run of 10:04 to 10:05 (load 4.4
falling, nothing else running); dur_parse and trim from 08:58 and
09:09. Short of a quiet --ab, an instruction ratio alone misread both
fault defer and RSA here: a single profiled run had native faster on
sign2048 (663 ms against 1,092 ms), and an 8 round run 1.39 slower.

Next for RSA: a mac_row that adds two rows a pass (each dst word loaded
and stored once per two rows); the earlier attempts that moved
mac_row's stores were slower on the wall (notes: stp pairing, grouped
stores).
