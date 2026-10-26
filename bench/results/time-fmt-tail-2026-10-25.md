# text.append_time's fraction tail without checks, 2026-10-25

The nanoseconds fraction of a formatted `Time` (`std/native/llrt/text.xo`,
`append_time`'s `ns != 0` branch) carried ten range checks: the trailing
zero countdown, `r = k - j`, `pow10`'s multiply, `digits_word(x / q, j) << 32`,
the shift and multiply inside `cat`, `r + 1`, `(4 + j) << 8` and
`(r + 1) << 12`. The tail now writes every bounded value so the backends'
value ranges prove it, and it has no checks left: arm64's `text.append_time`
goes from 703 to 543 lines of assembly.

## How it was measured

`bench/perf/text/xo` (the `time_fmt` and `dur_fmt` workloads) built with
`XO_INLINE_STRICT=1` by the tree's `xo`, arm64, five runs a column with
`/usr/bin/time -l`, the least instructions retired taken (they vary by 0.05%
between runs; wall is indicative only under load). The A column builds with
`--std=` a std whose `text.xo` is the one on origin/main (identical from
b7495cc8a on), B with the tree's; both use the same compiler, so the numbers
are the change's. LLVM is a third build of the same tree.

| time_fmt, 1,000 Times, 200 rounds | A (before) | B (after) | LLVM | B / A | B / LLVM |
|---|---|---|---|---|---|
| instructions retired | 152,186,300 | 142,235,040 | 145,540,626 | 0.934 | 0.977 |
| wall, µs | 5,989 | 5,671 | 7,710 | 0.95 | 0.74 |

| dur_fmt, 1,000 Durations, 200 rounds | A (before) | B (after) | LLVM | B / A | B / LLVM |
|---|---|---|---|---|---|
| instructions retired | 133,918,809 | 130,876,372 | 133,997,265 | 0.977 | 0.977 |
| wall, µs | 4,274 | 4,194 | 4,235 | 0.98 | 0.99 |

`dur_fmt` moves because `pow10` (the same file, `frac_words`) is now a tree
of ten constants instead of a multiply loop.

## Output

The checksum of each workload is the same in all three builds (`time_fmt`
826,361,646, `dur_fmt` 575,464,498). A 301,476 line sweep of formatted
Times and Durations (every value below 3,000 ns, every power of ten and its
neighbours, runs of trailing zeros from 1 to 9 significant digits times
10^0 to 10^8, 40,000 spread values, each under six second fields) hashes the
same from the before std and the after std (md5
c7dd52afeba197743e102a6a7b13ffcf). The time goldens pass on arm64 and LLVM
with Go parity:

`go test ./internal/arm64 -run 'TestGoldenRun/(time_fixed_offsets|time_parse_digits|floor_div_time|clock_timers|duration_parse_edges|rt_poll_timer|realtime_section)'`

`XO_TEST_GO_PARITY=1 go test ./internal/llvm -run 'TestGolden/(...)|TestGoldenGoParity/(...)'` (the same seven), and
`XO_WASM_GOLDENS='time_*,clock_timers,rt_poll_timer,realtime_section' go test ./internal/selfhost -run TestSelfhostWasm`.

## What changed

- `var k = 9` counts the trailing zeros down with `k = (k - 1) & 0xF`: the
  countdown cannot overflow, and k is still 1 to 9.
- `j = min(k, 4)` and `r = max(k - 4, 0)` are masked to 0 to 7, so
  `r + 1`, `(4 + j) << 8` and `(r + 1) << 12` are in range.
- `10^r` is a tree of eight constants written at the call site: through the
  `pow10` call the result had no range, so `x / q` checked for zero.
- `cat(digits_word(x % q, r), r, 90)` is written out, and
  `digits_word(x / q, j)` is masked with `0x7FFFFFFF` before `<< 32`.
- `pow10` is a tree of ten constants (the same ten values) for `frac_words`,
  which removes its multiply check and its callers' zero checks.

The masks change no value: the countdown runs at most 8 times, `j` is at
most 4, `r` at most 5, `digits_word` of 4 digits is below 10,000, and
`pow10` of 0 to 9 is the same ten constants.
