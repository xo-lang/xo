# Build log: 07-cache (strict Go)

Before run 1, a self review (no command run) moved a `t.Fatalf` reached from `wg.Go` goroutines (via `doRequest`) into a goroutine safe `fetchStatus` helper. No tool reported it, so it is not counted below.

| Run | Result | Distinct problems | Problems (category: description) |
|-----|--------|-------------------|----------------------------------|
| 1 | fail | 3 | nilaway: `service.go` FetchItem, result of `http.Client.Do` treated as possibly nil before `resp.StatusCode` / `resp.Body` use (false positive)<br>nilaway: `service_test.go` doRequest, same `http.Client.Do` nil flow on `resp.Body` / `resp.StatusCode` (false positive)<br>nilaway: `service_test.go` fetchStatus, same `http.Client.Do` nil flow (false positive) |
| 2 | pass | 0 | all checks ok (vet, staticcheck, errcheck, nilaway, exhaustive, test -race) |

Note: nilaway printed 2 grouped reports (14 dereference sites) from the single source `http.Client.Do` result 0. The table counts one problem per call site (3).
