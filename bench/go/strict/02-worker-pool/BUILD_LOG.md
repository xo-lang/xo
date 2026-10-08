# Build log

Every run of `strict-check.sh` on this folder.

| Run | Result | Distinct problems | Problems (category: description) |
|-----|--------|-------------------|----------------------------------|
| 1 | fail | 1 | nilaway: pool.go report loop indexed `jobs[i]` while ranging over `outcomes`; nilaway traced the literal `nil` jobs slice from TestEmptyJobs into that index (cannot actually panic, loop body never runs for an empty slice) |
| 2 | pass | 0 | none |

Totals: 2 runs, first run failed, 1 problem (nilaway: 1). vet, staticcheck, errcheck, exhaustive, and `go test -race` were ok on every run.
