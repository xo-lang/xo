# The Go backend against native: where it is slowest (2026-10-22)

Every bench/perf program that has a `measure.sh` taking backends, built
with `--backend=go` and `--backend=arm64` by the same xo (origin/main
a93ba88ff, its std), and bench/perf/xo's collection workloads. Instructions
retired from `/usr/bin/time -l`, the least of 5 runs; the program's empty
run (`none`, the setup) is subtracted from both sides where the program has
one. The collection rows (bench/perf/xo) have no instruction count: best of
5 wall times. Development Mac (arm64), a bulk slot (other jobs running), so
the times are noisy; the instruction counts are not. Checksums equal on both
backends in every row. Raw log: the lead's agents/gob-bench-cmd.log.

The Go backend's instructions include its runtime's other threads (the
garbage collector, the scheduler), which the native runtime does not have:
ratios here count everything the process retired.

## Ranking: the slowest ratios (Go backend over native)

| # | Workload | Go backend | native | Go / native |
|---|---|---|---|---|
| 1 | json parse | 5,594 M | 271 M | 23.8x |
| 2 | ctx mutex | 20,469 M | 1,103 M | 18.8x |
| 3 | atomic set, get, flag, cas | 169 to 349 M | 23 to 37 M | 12.4x to 16.6x |
| 4 | collection copymut | 47,959 us | 4,638 us | 10.3x |
| 5 | json invalid | 1,764 M | 204 M | 10.2x |
| 6 | atomic swap, contended | 257 M, 452 M | 37 M, 70 M | 9.9x, 7.5x |
| 7 | portable arith8, divmod, shift, arith32 | 247 to 844 M | 50 to 155 M | 5.3x to 6.1x |
| 8 | ctx spawn, await | 311 M, 382 M | 63 M, 82 M | 5.6x, 5.1x |

Of 101 workloads, the median ratio is about 2x; the Go backend is faster on
nine, most of them where native has its own gap (collection branch 0.003x:
212 us against 71,794 us; fsproc cwd 0.14x; collection insrem 0.22x; ctx
budget 0.63x; digest pbkdf2 0.70x), which are native items, not this one.

## 1. json parse: fixed

Cause (pprof of the generated program, `XO_CPUPROFILE`): the document has
one string with a `\u` escaped surrogate pair (an emoji, `\ud83d\ude00`),
and std/rt's fast JSON reader (jsonparsefast.go) gave up at any surrogate
escape, so the whole 100,000 token document went through
`json.Decoder.Token`, several allocations a token. The reader now decodes a
high surrogate followed by a `\u` low one as one rune, as the decoder does,
and still leaves a lone or reversed surrogate (the decoder's U+FFFD) to the
decoder.

| Workload | before | after | change |
|---|---|---|---|
| parse | 5,646 M (405 ms) | 1,648 M (121 ms) | -70.8% |
| small | 560 M | 561 M | 0% |
| strings | 887 M | 892 M | +0.5% |
| maps | 792 M | 795 M | +0.4% |
| invalid | 1,790 M | 1,790 M | 0% |

(Least of 5, a bulk slot, both sides built by the same xo with the std
before and after; the `none` setup run 74 M on both.) parse is now 6.8x
native (1,574 M against 233 M over setup). What remains: `json.decode_str`
to a type checks the text with `encoding/json.Valid` (40% of the profile)
and then builds the whole value tree (30%) to report that the array is not
a Bool; reading the text once (the fast reader's own validity) would cut
the first.

`xo bench --ab` (decides the speedup; 10 rounds, a bulk slot, the old and
new binaries above, argument `parse`, 2026-10-22 09:34):

| metric | before (median) | after (median) | after / before | 95% interval | verdict |
|---|---|---|---|---|---|
| wall_ms | 266.1 | 83.84 | 0.315 | 0.307 to 0.319 | better |
| cpu_ms | 260.0 | 70.0 | 0.269 | 0.264 to 0.269 | better |
| instructions | 5,574,934,230 | 1,634,315,290 | 0.293 | 0.293 to 0.293 | better |
| max_rss_kb | 39,512 | 38,336 | 0.97 | 0.963 to 0.981 | better |

## 2. ctx mutex: fixed

bench/perf/ctx `mutex`: 20,469 M against 1,103 M native. Cause (pprof of
the generated program): every `Mutex.with` recorded where it was taken
for the re-entrancy and deadlock reports, as `runtime.Callers` over 14
frames into a 14-slot site, and that walk was 64% of the instructions.
The caller of `Mutex_with` is generated code, whose first frame is the Xo
frame the report names, so the site now keeps two frames (std/rt
sync.go heldSite).

`xo bench --ab` (decides; 10 rounds, a bulk slot, both sides built by the
same xo with the std before and after, argument `mutex`, 2026-10-22
09:34):

| metric | before (median) | after (median) | after / before | 95% interval | verdict |
|---|---|---|---|---|---|
| wall_ms | 903.1 | 300.9 | 0.333 | 0.309 to 0.359 | better |
| cpu_ms | 890.0 | 280.0 | 0.315 | 0.308 to 0.331 | better |
| instructions | 20,462,681,357 | 7,271,319,606 | 0.355 | 0.355 to 0.357 | better |
| max_rss_kb | 26,984 | 26,960 | 0.999 | 0.996 to 1.001 | within noise |

ctx mutex is now 6.6x native (7,271 M against 1,103 M).

## 3. atomic: fixed

Cause (the generated Go and its disassembly): std/rt's `Atomic[T]` was a
`sync.Mutex` and a value, with a `defer` per operation, and every
operation first called `Task.Yield`, whose cost (82) was over Go's
inlining budget (80). `Atomic` is now one `atomic.Int64` (a Bool as 0 or
1; the encoding is an `unsafe.Sizeof` test that folds in each of the two
instantiations, so `get` is one `LDAR`), `add` and `max` are compare and
swap loops, and `Yield` inlines its test of the deterministic scheduler
(the yield itself is a separate function).

Instructions over setup (least of 3; `none` 35.4 M on both sides):

| Workload | before | after | change |
|---|---|---|---|
| get | 205.8 M | 81.1 M | -60.6% |
| set | 132.7 M | 62.8 M | -52.7% |
| swap | 220.8 M | 108.5 M | -50.9% |
| cas | 274.5 M | 94.6 M | -65.5% |
| add | 256.5 M | 160.4 M | -37.4% |
| max | 190.6 M | 70.3 M | -63.1% |
| flag | 312.4 M | 132.8 M | -57.5% |

get is now 6.8x native (81 M against 12 M over setup). The checksums are
the same on both sides for every workload.

`xo bench --ab` (decides; 10 rounds, the binaries above, 2026-10-22
09:34; `set` again without cpu_ms, see below):

| Workload | wall_ms before | after | after / before | 95% interval | instructions after / before | verdict |
|---|---|---|---|---|---|---|
| get | 16.43 | 8.348 | 0.508 | 0.46 to 0.575 | 0.482 | better |
| set | 12.15 | 7.701 | 0.634 | 0.602 to 0.829 | 0.584 | better |
| cas | 18.41 | 9.84 | 0.534 | 0.495 to 0.617 | 0.419 | better |
| add | 17.6 | 16.15 | 0.918 | 0.883 to 0.967 | 0.672 | better |
| flag | 20.85 | 11.8 | 0.566 | 0.535 to 0.626 | 0.483 | better |
| contended (4 tasks) | 144.4 | 150.8 | 1.044 | 0.778 to 1.111 | 0.266 | within noise |

contended's wall time is the scheduling of four goroutines on one value,
not the operation; its instructions fell to 0.27.

`xo bench --ab` faulted on `set` with `cpu_ms` included ("cannot convert
NaN to Int"): both medians were 0 (rusage's 10 ms resolution) and the
ratio 0/0 reached the formatter. The cpu_ms rows above it (10 against 0,
"better") are below the metric's resolution and say nothing.

## Not measured

gen, glob, report, testworld: their measure.sh build `xo test --no-gc`,
which needs a native backend. rsa and fsproc have no `none` row (the
ratio there is of totals). The http, httpclient, tls, net, sysio, fault,
stdio programs need servers or other arguments and were not run.
