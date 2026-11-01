# 0340: `xo bench --ab`, an interleaved A/B measurement with a confidence interval

Status: Accepted (owner request, 2026-10-22, through the lead: build it; shipped as `xo bench --ab`, compiler/cli/bench, and required for every speedup claim)

## Context

Every performance decision in this repository (0077, 0286, 0324, 0329,
0330, the ghash4 revert) rests on a hand written script: build two
variants, run them in turn, take a median, say "within noise" by eye.
The scripts differ in rounds, in what they interleave and in what they
record, none states an interval, and the load of the machine is a
side remark ("loaded, ratios only"). The ghash4 call sites (callgrind
-12.6% instructions, wall throughput -24%) and the Go rows that swing
2x under load show why the verdict has to come from the spread of the
measurements and not from one number. Go's benchstat does this for
`go test -bench`; Xo has nothing for whole programs, servers and load
generators.

## Decision

A new command, `xo bench`, whose first mode is `--ab`:

    xo bench --ab [flags] A [B] [-- ARGS...]

**What is compared.** A and B are programs (paths, or names on PATH).
When B is omitted it is A, so one build under two settings is
`xo bench --ab ./srv --env-b XO_THREADS=1`. Per side: `--env-a K=V`,
`--env-b K=V` (repeatable), `--args-a "..."`, `--args-b "..."`; the
arguments after `--` go to both. A program that needs a server and a
load generator is a script (`A` is `sh`, `--args-a "run-a.sh"`); the
measured process is then the script and everything it started and
waited for.

**Rounds.** `--rounds N` (default 10, 3 since the 2026-11-01 addendum) rounds after `--warmup W`
(default 1, discarded). Each round runs A and B once, back to back; the
order alternates by round (A B, B A, ...) so a drift, a thermal slope or
a cache warmup does not favor a side. Per round and per side the
command records the metrics below and the load before and after.

**Load guard.** `--max-load X` waits before each run until the 1 minute
load average is under X (poll every 5 s, up to `--load-wait`, default
10 min; then the round runs and is marked `loaded`). `--guard CMD`
generalizes it: a run starts only when `CMD` (run by `sh -c`) exits 0,
for a quiet-window file or a CPU idle check. Both can be given. The
load before and after every run is in the output and the JSON, so a
result can say "all rounds under load 8".

**Metrics.**

| metric | source | better |
|---|---|---|
| `wall_ms` | monotonic time around the run | lower |
| `cpu_ms` | user plus system time of the process and its waited-for children | lower |
| `cpu_us_per_op` | `cpu_ms` over ops | lower |
| `instructions` (and `per_op`) | macOS: instructions retired of the run; Linux: perf_event when the kernel allows it; absent otherwise | lower |
| `max_rss_kb` | the largest resident set of the run | lower |
| any number the program reports | the last JSON object line on its stdout | by name |

*ops* is `--ops N`, else the program's reported `ops`, `requests` or
`all_requests`. A reported key is compared like the built in ones; its
direction is higher-is-better when the name has `rps`, `mbps`, `per_sec`
or `throughput`, else lower-is-better (`_us`, `_ms`, `_kb`, `p99`, ...).
`--metrics a,b,c` restricts the table.

**Statistics.** For each metric the rounds give paired values (a_i,
b_i). The report is:

- the median of each side and its spread (the interquartile range as a
  percentage of the median);
- the ratio of the medians B/A and a 95% confidence interval from a
  percentile bootstrap that resamples whole rounds (10,000 resamples,
  deterministic from `--seed`, default 1), so the pairing and the
  interleaving are kept;
- the verdict: **better** or **worse** when the interval excludes 1.0
  (direction from the table), else **within noise**; **too few rounds**
  below 3 valid rounds (5 until the 2026-10-31 addendum). `--alpha` changes the level (default 0.05).

A round marked `loaded` is excluded from the statistics by default
(`--keep-loaded` includes it), and the report says how many rounds were
used. A run that exits nonzero or times out (`--timeout`, default
10 min) fails the command (exit 1) unless `--allow-fail`.

**Output.** A table for people, `--json` for agents (one object: the
parameters, every round's raw values and loads, and per metric the
medians, spreads, ratio, interval, verdict). Exit 0 on a completed
comparison whatever the verdict (the verdict is data), 1 on failed
runs, 2 on a usage error.

**Implementation.** `compiler/cli/bench` (statistics in `stats.xo`, with
unit tests of the median, quantile, bootstrap and verdict on known
data; process running in `run.xo`), dispatched from `compiler/xo/main.xo`
as `xo bench`. Stage 1 gets CPU, RSS and instructions without a runtime
change by running each measured process under the platform's `time`
(`/usr/bin/time -l` on macOS, which reports user, system, maximum
resident set and instructions retired; GNU `/usr/bin/time -v` on Linux,
installed in `scripts/docker-linux.sh`'s image, with no instruction
count there). Stage 2 replaces the wrapper with `Child.usage()` in std
(wait4's rusage, `proc_pid_rusage` on macOS, perf_event on Linux), a
std.md 3.8 addition with its own change log and the three backends'
lowering; it is a separate decision when the wrapper's limits (the
extra exec, the missing Linux instruction count) matter.

**Default guard (lead, 2026-10-22).** Without `--no-guard` and `--guard`
a run first waits while the quiet window file (`XO_QUIET_FILE`, default
`~/xo-lead/quiet`) holds a future epoch, so an A/B never adds load to
another session's measurement; the per-round load is printed beside the
table.

**Implemented (2026-10-22).** `compiler/cli/bench/{bench,stats}.xo` with
tests; `xo bench` in `xo help`; the Docker image gains GNU `time` (tag
`-4`). Resolution: `time` prints CPU in centiseconds, so a run of under
about 100 ms has a coarse `cpu_ms`.

## Consequences

- One tool and one verdict rule for the perf decisions; a result file
  can quote the command and its JSON.
- Xo Lang 2's spin and Mutex A/Bs run as `xo bench --ab` with
  `--env-b`; the scripts that interleave by hand stay as they are until
  someone moves them.
- The wrapper adds an exec (about a millisecond) to each run and reports
  the wrapper's own CPU inside the child's; negligible for runs of
  hundreds of milliseconds and more, and stated in the output header for
  shorter ones.
- Instructions exist on the Mac only in stage 1; the Docker Linux path
  compares CPU, RSS and wall.
- A bootstrap interval from 10 rounds is wide on a noisy machine; that
  is the point (the report says "within noise" instead of a ratio), and
  `--rounds` or the load guard is the remedy.

## Alternatives

- **benchstat itself** on a text file of results: needs Go and the
  `go test -bench` format, and the metrics here are not Go benchmark
  lines.
- **A Mann-Whitney U test** (benchstat's choice) instead of the paired
  bootstrap: valid for unpaired samples; the interleaved design gives
  pairs, and the interval on the ratio is what the decisions quote.
- **`Child.usage()` first:** the better end state, but a std API, three
  backends and a spec change before the tool exists; the wrapper gets
  the tool in use first.
- **Extend `xo dev compare` (callgrind):** it counts instructions
  exactly but says nothing of wall, RSS or contention, which is where
  0077, 0286 and the ghash4 revert were decided.

## Addendum 2026-10-31: three rounds

The owner, 2026-10-31: "10 rounds sounds way too many", then "Do 3
rounds". MIN_ROUNDS is 3 (was 5) and ab-host.sh runs 3 rounds after one
warmup. With 3 paired rounds the bootstrap interval spans about the
rounds' own spread, so a verdict of better or worse needs all three
rounds on one side of 1.0; on a change with no effect that happens by
chance about one time in four (one in eight for each direction). A
verdict on an effect under about 3%, and any "no slower" claim, is
confirmed with `--rounds=10`; a large effect (the tail merge's 0.812,
whose 10-round interval was 0.808 to 0.815) needs no more.

## Addendum 2026-10-31: early stopping (--early)

Owner, 2026-10-31 ("6 yes"): the 10-round confirmations (effects under
about 3%, "no slower" claims) held the arm64 host 20 to 30 minutes each.
`xo bench --ab --early` treats --rounds as the most rounds. From round 5
(EARLY_MIN) it looks after every usable round: each watched metric
(--metrics, else wall_ms, cpu_ms, max_rss_kb; a too coarse one is skipped)
is decided "better" or "worse" when its interval excludes 1, or "same"
when the interval lies within 1 +- 1% (EARLY_EQ), and the run stops when
every one is decided with the same answers at two looks in a row. Each
look uses alpha divided by the number of looks (Bonferroni), so stopping
early keeps the false verdict rate within alpha. The output and the JSON
(`stopped_early`) say where it stopped. ab-host.sh passes --early for more
than 5 rounds when NEW's xo bench has it. A 3-round run never stops early.
First run (darwin, /bin/sleep 0.05 against 0.10, --rounds=12): stopped
after 6 rounds, wall 1.807 worse, max RSS the same, CPU too coarse.

## Addendum 2026-10-31: 3 rounds for every claim

Owner, 2026-10-31 ("I thought all tests were dropped to 3?"): the
10-round confirmation for effects under about 3% and "no slower" claims,
added with the three rounds addendum, is withdrawn; every --ab runs 3
rounds. A no-effect change still shows three agreeing rounds about one
time in four, so a 3-round verdict on a small effect is reported as such.
--early stays for a run given more than 5 rounds by hand.

## Addendum 2026-11-01: --rounds defaults to 3

Owner's rule, through the lead, 2026-11-01: a direct `xo bench --ab` with no
`--rounds` ran 10 rounds, 16 minutes of a Mac quiet window for three programs
that 3 rounds would have answered. The default is now 3, as ab-host.sh's has been.
--early's rule holds: its first look is round 5 and it needs two looks in a row,
so it can stop only a run above 5 rounds, and `--early` alone runs 3 rounds and
never stops early. A longer run still names `--rounds`.
