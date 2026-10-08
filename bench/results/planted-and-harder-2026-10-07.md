# Planted bugs and harder tasks (2026-10-07)

## Part 1: planted bug detection (Go arms, measured)

50 single bug variants (10 per task, tasks 01 to 05) planted by agents into
the plain Go solutions, non test code only, chosen for realism without
running any checks. Each variant ran through the plain loop (build, vet,
test) and the strict tools; a strict tool counts only for findings the
unmodified base does not produce. Raw data: `bench/planted/results.jsonl`,
bug descriptions in each `manifest.json`.

| Class | Plain caught | Plain + strict caught | Escaped both |
|---|---|---|---|
| X1 ignored error | 1/5 | 1/5 | 4 |
| X2 nil dereference | 4/5 | 4/5 | 1 |
| X3 missing case | 1/5 | 2/5 | 3 |
| X4 data race | 0/5 | 2/5 (race detector) | 3 |
| X5 goroutine or resource leak | 0/5 | 0/5 | 5 |
| X6 lost update | 3/5 | 4/5 | 1 |
| **X total (xo targets)** | **9/30 (30%)** | **13/30 (43%)** | **17** |
| N1 off by one | 5/5 | 5/5 | 0 |
| N2 wrong status or kind | 5/5 | 5/5 | 0 |
| N3 wrong arithmetic | 3/5 | 3/5 | 2 |
| N4 wrong condition | 4/5 | 4/5 | 1 |
| **N total (not xo targets)** | **17/20 (85%)** | **17/20 (85%)** | **3** |

Adjustments: the task 05 X3 staticcheck hit is the S1000 style warning on the
leftover single case `select`, not a detection, so it is excluded.

Findings:

1. **Agent written tests catch logic bugs well (85%) and safety bugs poorly
   (30%).** The bugs xo targets are the ones tests miss.
2. **Strict analyzers add 4 detections out of 30** (2 races via `-race`, 1
   exhaustive, 1 staticcheck on an ineffective value receiver write), at the
   cost of 3 false positives per 5 tasks seen earlier.
3. **Leaks escaped every tool: 5 of 5.**
4. **Ignored errors escaped 4 of 5**, and all 4 were explicit `_` discards,
   which errcheck does not flag by default.

## Part 2: what xo would do with the same 30 bugs (estimate, not measured)

The xo checker does not exist yet, so this is an analysis of each planted bug
against spec 0.3 rules. It will be replaced by measurement once the planted
bug test in `internal/check` and xo versions of the bases exist.

| Class | xo at compile time | Notes |
|---|---|---|
| X1 ignored error | 0/5 | All 4 escaped bugs are explicit `_` discards. The xo equivalents (`let _ =`, `.ok() ?? default`) are equally explicit and compile. xo only prevents implicit drops. |
| X2 nil | 5/5 | No nil. |
| X3 missing case | 3/5 | The 3 real closed set switches become exhaustive `match`. The 2 `select` case removals are not prevented. |
| X4 data race | 5/5 | Not expressible: no shared mutable values, `Mutex.with` scopes the lock, no pointers for lazy defaults. |
| X5 leak | 0/5 compile | 4 goroutine leaks become **hangs** (the scope waits for the blocked task), which is visible but only at run time, and only if a test triggers it. 1 is a file handle leak not applicable to xo's whole file API. |
| X6 lost update | 4/5 | 3 value receiver writes are rejected (mutation needs a `var` receiver); 1 missing map write back is XO0520. The 1 miss is a local copy mutated after being appended, which the 0.3 narrowing of XO0520 (decision 0023) no longer covers. |
| **X total** | **17/30 at compile time** | Union with tests that also caught them in Go: about 12/30 would escape, against 17/30 for strict Go and 21/30 for plain Go. |

Lessons for the design:

- **Error ignoring:** xo matches Go here. Idea: require a reason string to
  discard an error that carries a value (`r.ignore_err("best effort")`) so
  discards are searchable and reviewable.
- **Leaks:** structured concurrency converts leaks to hangs. A deadlock
  detector (P2) and a test runner timeout that prints the task tree would
  make those hangs actionable.
- **Decision 0023 cost:** the narrowed XO0520 misses one realistic lost
  update. Consider covering any local `var` that is mutated after being
  copied into a collection.

## Part 3: harder tasks 06 to 08

| Task | Plain Go runs | Strict Go runs | Strict findings | xo paper GUESS | Go lines (plain) | xo lines |
|---|---|---|---|---|---|---|
| 06 Job queue | 2 (own test helper bug) | 2 | nilaway 1 (false positive) | 4 | 1270 | 648 |
| 07 Cache | 1 | 2 | nilaway 3 (false positives) | 6 | 1200 | 599 |
| 08 Notes service | 1 | 1 | none | 7 | 1782 | 953 |

Plain Go solutions for 06 to 08 passed 5 runs each of `go test -race` with no
race, and the strict tools found only noise (unchecked `Close`, the nilaway
`http.Client.Do` false positive, one unused value in a test). All 32 xo files
for v0.3 (tasks 01 to 08) parse with the P1 parser with 0 errors.

xo design costs exposed only by the harder tasks:

1. **Fire and forget work under structured concurrency (07).** `get` cannot
   start a background refresh because a task cannot outlive its scope and a
   scope handle cannot be stored. The agent routed refreshes through a channel
   to a daemon task; there is no non blocking send. Evidence against 0007 as
   specified; a fix is a long lived scope owned by the cache
   (`task.Group` passed in at construction) plus `try_send`.
2. **Effects leak into generic types (07, 06).** A cache over an effectful
   loader becomes `Cache[Int, Item, UpstreamErr, net.dial]`; handlers with
   effect variables repeat long signatures; no struct can hold an effect
   generic function. Evidence against 0004 as specified.
3. **`Mutex.with` rollback on error (06).** Changes are discarded when the
   closure returns `Err`, so "expire leases, then reject a stale token"
   needs two critical sections. Workaround: return `Result` as the success
   value and apply `?` outside; the spec should show it.
4. **No streaming HTTP bodies (08)**, no wait for N tasks with a deadline
   (06), `fail_next` needs exact paths (08).
5. Go needed about twice as many lines; part of this is more tests in Go.
   Not a quality measure until the xo versions run.

## Next experiments

1. When `internal/check` lands: run it on `bench/paper/v0.3/` and on the
   planted bug xo programs, replacing Part 2 estimates with measurements.
2. Translate the 5 Go bases to xo (once the checker passes them) and plant the
   same 30 X bugs in xo form, to measure rather than estimate.
3. Spec 0.4 candidates from this run: error discard with reason, long lived
   task groups and `try_send`, effect elision in generic types (infer a
   type's effect parameter from its constructor), document the `Mutex.with`
   Result pattern, streaming bodies, XO0520 coverage for post append copies.
