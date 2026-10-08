# Go baseline: plain Go and strict Go (2026-10-07)

Stage 3 arms 1 and 2. Ten fresh agents, the same five tasks, Go 1.26.1,
standard library only. Each wrote the full solution before running anything,
then iterated. Solutions and `BUILD_LOG.md` files are in `bench/go/<arm>/<task>/`.
The xo arm cannot run until the compiler exists, so this is a baseline, not a
comparison yet.

## Results

| Task | Plain runs | Plain first run | Strict runs | Strict first run | Strict findings during the loop |
|---|---|---|---|---|---|
| 01 CRUD API | 1 | pass | 1 | pass | none |
| 02 Worker pool | 1 | pass | 2 | fail | nilaway 1 (false positive) |
| 03 Retry client | 1 | pass | 2 | fail | nilaway 1 (true positive: nil response from a custom `Doer`) |
| 04 File sync | 1 | pass | 2 | fail | nilaway 1 (false positive) |
| 05 Rate limiter | 1 | pass | 2 | fail | own tests 2 (one real logic bug: per key limits never applied); nilaway 1 (false positive) |
| **Total** | **5** | **5/5** | **9** | **1/5** | **real bugs: 2, false positives: 3** |

Go lines (code plus tests): plain 3451, strict 3721. The agents wrote many
more tests than required in both arms.

Verification: I reran `strict-check.sh` on all ten folders. All five strict
solutions pass every check, matching their logs.

## Strict checks applied to the plain arm (escaped findings)

| Task | Findings | Real bugs |
|---|---|---|
| 01 | errcheck 2 (`resp.Body.Close` in tests), nilaway 1 (`http.Client.Do` result) | 0 |
| 02 | errcheck 1 (`defer resp.Body.Close`) | 0 found by tools (see below) |
| 03 | errcheck 1, nilaway 1 (nil response from a custom `Doer`), exhaustive 1 (intentionally partial switch in `Error()`) | 1 latent |
| 04 | errcheck 12 (Fprintf to stderr, deferred Close, Close on an error path), nilaway 1 (nil slice, safe) | 0 |
| 05 | errcheck 1 (test) | 0 |

Of about 24 findings, 1 is a latent real bug. The rest are conventional Go
(unchecked `Close`, `Fprintf`) or false positives.

## Bugs no tool caught

- **Goroutine leak by design (plain 02).** On timeout `Run` returns without
  waiting for workers that ignore their context. Tests pass, every analyzer
  passes. The strict 02 agent chose to wait for workers. xo makes this
  impossible: no task outlives its scope.

## What this means for xo

1. **Go is a strong baseline for agents on these tasks.** Plain Go passed
   5/5 on the first run. Any first try pass rate advantage for xo is capped.
2. **Strict analyzers mostly add noise.** 1 true positive in the loop and 1
   latent bug in escaped findings, against 3 loop false positives and about
   23 escaped noise findings. They cost 4 extra runs.
3. **xo's case must rest on bugs tests do not catch.** In this sample: one
   nil flow and one goroutine leak, both classes xo rules out by
   construction. Five tasks is too small a sample to size the effect.
4. **The current tasks are too easy to separate the arms.** Next: harder
   tasks with concurrency and failure paths, plus planted bug detection.

## Next experiments

1. Planted bug benchmark: for each Go solution, plant 5 bugs from classes xo
   prevents (nil map write, ignored error, missing enum case, data race,
   goroutine leak, wrong platform branch) and 5 it does not (off by one, wrong
   status code, wrong timeout math). Measure which arm's tooling or tests
   catch each, and how fast a fresh agent finds the root cause.
2. Harder tasks 06 to 08: a job queue with retries and leases, a cache with
   singleflight and eviction under concurrency, a multi module service
   (also tests decision 0006).
3. The xo arm, once `xo check` and `xo test` run.
