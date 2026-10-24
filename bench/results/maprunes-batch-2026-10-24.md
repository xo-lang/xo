# map_runes: a rune that maps to itself keeps its own bytes, 2026-10-24

`Str.map_runes` (Go's `strings.Map`) copied the receiver until the first
rune that changed, then pushed every remaining rune one at a time through
`raw.push_rune`. A CPU profile of the `mapfunc` workload (200 Hz, 44
samples) put half of the mapping time under `raw.push_rune`
(`cabi.obj_rs_push_byte`, `obj.rs_push_byte`, `obj.rs_push_word`,
`obj.reserve`), against 20 percent in the two closures and 9 percent in
decoding: the append path, not the predicate, was the gap to Go, whose
`WriteByte` is inlined and whose `strings.Builder` writes byte by byte
into one buffer.

The rest of the mapping now keeps a rune that maps to itself as its own
bytes and flushes a run of them with one `rt.rs_append_bytes`, and pushes
an ASCII result with `rt.rs_push_byte` rather than through
`raw.push_rune`'s scalar checks. `f` is still called exactly once per
rune and the output is byte for byte Go's: the corpus (`strwhere`, Go's
`TestMap` rows and 2,000 seeded randoms with invalid UTF-8, on arm64,
LLVM and the Go backend) and the golden `strwhere` (arm64, LLVM, Go
parity, wasm) pass unchanged.

`xo bench --ab --rounds=12` on the `mapfunc` binary built before and
after the change, same program, interleaved:

| metric | A (before) | B (after) | B / A | 95% interval | verdict |
|---|---|---|---|---|---|
| instructions | 5,939 M | 4,241 M | 0.714 | 0.714 to 0.714 | better |
| wall_ms | 282.3 | 217.8 | 0.771 | 0.751 to 0.819 | better |
| cpu_ms | 270.0 | 210.0 | 0.778 | 0.750 to 0.778 | better |
| max_rss_kb | 7,328 | 7,304 | 0.997 | 0.996 to 1.000 | within noise |

The machine was loaded (1 minute load 28 to 33), so instructions decide
and wall time is the shape, not the number.

Against Go, measured in the same run as the row below (`measurego.sh`,
same host, same day): mapfunc arm64 4,240 M, LLVM 4,485 M, Go 4,958 M,
so 0.86 of Go's instructions on arm64 and 0.90 on LLVM, from 1.20 and
1.38 in `str-func-2026-10-23.md`.

Reproduce:

```
bench/perf/unicode/measure.sh "$PWD/bin/xo" /tmp/u arm64 llvm
bench/perf/unicode/measurego.sh /tmp/u
xo bench --ab --rounds=12 /tmp/ab-old/u-old /tmp/ab-new/u-new -- mapfunc
```

where `u-old` is built by an xo at the parent commit and `u-new` by this
one, both from a copy of `bench/perf/unicode`.
