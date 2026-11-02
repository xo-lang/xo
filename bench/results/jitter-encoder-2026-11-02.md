# /alloc's CPU: Go's encoder against Xo (2026-11-02, Xo Lang 4)

Decision 0324's option 3: how much of Xo's CPU lead on bench/perf/jitter's
`/alloc` comes from Go's reflecting `encoding/json`. Go routes on the same
server (6886a6ad8): `/alloc` (fmt.Sprintf and json.Marshal), `/alloc-strconv`
(strconv, json.Marshal), `/alloc-hand` (strconv and a hand-written encoder,
byte for byte json.Marshal's output: TestAppendPage). Xo `/alloc` is its
derived encoder. linux/arm64 first host, open loop, 64 connections, 10 s, 3
rounds rotated; every body 14,683 bytes, no errors. Median (range).

## CPU us a request

| rate/s | Go /alloc | Go /alloc-strconv | Go /alloc-hand | Xo /alloc |
|---:|---:|---:|---:|---:|
| 5,000 | 171 (171 to 173) | 140 (137 to 146) | 114 (114 to 115) | 67 (66 to 68) |
| 10,000 | 168 (167 to 169) | 137 (136 to 142) | 119 (118 to 121) | 60 (59 to 60) |
| 20,000 | 194 (190 to 195) | 152 (141 to 152) | 126 (122 to 126) | 59 (59 to 60) |

Formatting is about 30 us of Go's cost and reflection about 25 us more; the
hand-written Go still takes 1.9x to 2.1x Xo's CPU, so most of the gap is
outside the encoder (net/http, GC, building the page; not broken down).

## Latency p50 / p99 us

These are totals from a generator without `-awake`: at 5,000 and 10,000 a
second most of each figure is the generator's own lateness, not the
server's (see the note below the table). The CPU table above is the
server's process alone, with arrivals bunched by the late generator; with
`-awake`, Xo's /alloc CPU a request at 20,000 a second is about 69 us
instead of 59 and Go's about the same (jitter-low-rate-p50-2026-11-02.md),
so Go hand-written is about 1.8x Xo there, not 2.1x.

| rate/s | Go /alloc | Go /alloc-strconv | Go /alloc-hand | Xo /alloc |
|---:|---|---|---|---|
| 5,000 | 672 / 1,964 | 686 / 1,853 | 697 / 1,781 | 713 / 1,624 |
| 10,000 | 203 / 1,625 | 201 / 2,127 | 554 / 2,176 | 584 / 1,711 |
| 20,000 | 1,100 / 13,953 | 333 / 1,044 | 244 / 743 | 113 / 193 |

At 20,000 Go's `/alloc` saturates.

A comparison of Xo with Go on this bench uses the `/alloc-hand` row: it is the
fair baseline against Xo's generated encoder.

The higher p50 at low rates (5,000 and 10,000 against 20,000), on Go's
hand-written route as well as Xo's, was the load generator's own timer:
explained and fixed in jitter-low-rate-p50-2026-11-02.md (loadgen -awake).
