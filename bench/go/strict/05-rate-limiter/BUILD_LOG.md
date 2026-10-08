# Build log

| Run | Result | Distinct problems | Problems |
|-----|--------|-------------------|----------|
| 1 | fail | 3 | (1) test-failure: TestRetryAfter, custom limits for keys "slow"/"tiny" never applied, so no 429 and empty Retry-After, because ClientKey namespaces keys as "key:<value>" while limitFor looked up the config map with the prefixed key. (2) test-failure: TestCustomLimit, "vip" got 200 on 4th request, same root cause as (1) (counted separately because it surfaced in a separate test). (3) nilaway: limiter_test.go TestServeStops dereferenced `resp` from http.Get without a nil check (3 sites, one nil source). |
| 2 | pass | 0 | all checks ok |

Notes on counting: problems (1) and (2) share one root cause (one bug in limitFor). By root cause, run 1 had 2 distinct bugs: 1 test-failure bug and 1 nilaway finding.

Before run 1 (no command had been run yet other than `mkdir` and `go mod init`), I removed two pieces of leftover test scaffolding by hand: an unused `_ = h` limiter in TestKeyFallsBackToRemoteIP and a JSON based `itoa` helper replaced with `strconv.Itoa`. No tool flagged these; they were edited before the first check.
