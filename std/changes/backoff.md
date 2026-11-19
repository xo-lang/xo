# std/backoff changes

## 2026-11-18: fix: an interval near Int's largest is Int's largest after jitter, not a fault

- Kind: fix
- Commit: this commit
- Found by: TestBackoffFuzz with the hostile dictionary (task #352)
- Go reference: cenkalti/backoff v4 exponential.go getRandomValueFromInterval and incrementCurrentInterval (MIT license), whose time.Duration(float64) gives Int's largest on arm64
- Test: std/backoff's "a randomized interval past Int's largest is Int's largest, as cenkalti/backoff's on arm64"; TestBackoffFuzz
- Shipped in: not yet released

A delay whose randomized interval passed Int's largest nanoseconds (an initial or max interval near 292 years) faulted with
"out of range for Int"; it is Int's largest now, as Go's conversion on arm64 (amd64's gives Int's smallest, which Xo does not copy).

## 2026-11-16: behavior: `ticker` takes a `Rand[rand]` and `Ticker.rand` is a `Rand[rand]` (decision 0652)

- Kind: behavior
- Commit: this push
- Found by: task #141, Rand's effect parameter (decision 0652)
- Go reference: none (Xo's effect system)
- Test: std module tests; golden rand_pure
- Shipped in: not yet released

Rand takes its effect as a parameter now; these hold the OS generator, so they are written with the effect they had before. A `Rand[()]` (a pure one) is still accepted.

## 2026-11-16: fix: a multiplier below 1 shrinks the interval after the cap

- Kind: fix
- Commit: this push
- Found by: TestBackoffFuzz (task #116), a differential against cenkalti/backoff v4's ExponentialBackOff arithmetic on random schedules
- Go reference: cenkalti/backoff v4.3.0 exponential.go incrementCurrentInterval (Copyright (c) 2014 Cenk Altı, MIT license)
- Test: std/backoff_test.xo "a multiplier below 1 shrinks a capped interval"; internal/stdtest/testdata/corpus/backofffuzz (seed 1, 400 schedules); TestBackoffFuzz seeds 2 and 3, 5000 schedules each (arm64)
- Shipped in: not yet released

`delay(attempt, rand)` stopped growing the interval once it reached `max_interval`. Go's incrementCurrentInterval multiplies a capped interval again, so a multiplier below 1 (or 0) shrinks it: initial 10s, multiplier 0.5, max 4s gives 10s, 4s, 2s, 1s, 500ms, where Xo gave 10s, 4s, 4s, 4s. The walk now ends only at a fixed point.

## 2026-11-15: feature: ticker, Ticker.next and Ticker.stop (cenkalti/backoff v4's Ticker)

- Kind: feature
- Commit: this push
- Found by: scripts/stdaudit gaps backoff.NewTicker, NewTickerWithTimer, Ticker and Ticker.Stop (decision 0655)
- Go reference: cenkalti/backoff v4.3.0 ticker.go and ticker_test.go (Copyright (c) 2014 Cenk Altı, MIT license)
- Test: std/backoff_test.xo on the fake clock: TestTicker's three ticks, TestTickerContext's first tick, the max_elapsed and max_tries ends, a late read, jitter bounds (arm64, llvm)
- Shipped in: not yet released

A pulled ticker: `next()` sleeps until the tick is due and answers its time, None once the schedule ends or `stop` ran; the delay to the next tick is drawn when a tick is given, as Go's goroutine does after a send.

## 2026-11-11: refactor: std/backoff is one file (decisions 0588, 0597)

- Kind: refactor
- Commit: this push
- Found by: decision 0597's merges
- Go reference: none (no behavior change)
- Test: TestStdCorpora norm, backoff, idna, otlp on go, arm64, llvm; golden otlp_export (arm64, llvm, Go parity); TestStdChecks; repotest
- Shipped in: not yet released

std/native/backoff.xo moved into std/backoff.xo; backoff.retry and retry_notify left the compiler's list of bodyless std functions that do not wait (compiler/ssa/waits.xo), since they have Xo bodies now. It is generated into std/rt (std/rt_hosted.txt), since std/otlp, which is, uses it.

## 2026-10-30: feature: retry_notify

- Kind: feature
- Commit: this commit
- Found by: Xo Lang 1's triage of the 2026-10-30 language obstacles note
  (grpcsvc wanted to log each retry)
- Go reference: cenkalti/backoff v4 RetryNotify
- Test: internal/stdtest/testdata/corpus/backoff (TestRetryNotify, and
  no notify for an answered error), every backend
- Timing: none of its own: retry's loop plus one call per retry

`backoff.retry_notify(b, clock, rand, op, retryable, notify)` calls
`notify(e, d)` before each sleep, so a program can log or count retries;
its `retryable` may have effects.

## 2026-10-29: change: the Go backend compiles the native port

- Kind: change
- Commit: this commit
- Found by: std/rt/backoff.go was a hand written twin while the Go
  backend instantiated a compiled generic std function with Unit (fixed
  in 989661bf8)
- Go reference: none (same behavior)
- Test: internal/stdtest/testdata/corpus/backoff on the Go backend, the
  first go_compiled module with a generic free function (retry, always)

std/backoff joins std/go_compiled.txt: the Go backend compiles
std/native/backoff.xo, and std/rt/backoff.go is gone.

## 2026-10-29: port: std/backoff, cenkalti/backoff v4

- Kind: port
- Commit: this commit
- Found by: gap 15 of the 2026-10-29 std survey (retry loops with
  hand rolled sleeps); API approved by the lead 2026-10-29 (clock and
  rand per call, retryable required, `always()`)
- Go reference: github.com/cenkalti/backoff/v4 exponential.go
  (NextBackOff, incrementCurrentInterval, getRandomValueFromInterval),
  retry.go, and their tests
- Test: internal/stdtest/testdata/corpus/backoff, generated by
  scripts/stdcorpus/backoff.go (the delays computed by v4's arithmetic
  for scripted random values; the retry cases on the test's FakeClock),
  on arm64, LLVM and the Go backend (std/rt/backoff.go until the entry
  above)
- Timing: none: retry is sleep bound and delay is a few float
  operations

`backoff.retry(b, os.clock, os.rand, op, backoff.always())` retries op
with exponential delays and jitter; `Backoff` holds cenkalti's defaults.
