# Build log

| Run | Result | Distinct problems | Problems |
|-----|--------|-------------------|----------|
| 1 | fail | 1 | nilaway: `resp.Body` accessed at retry.go:258 when the `Doer.Do` interface result could be nil with a nil error (a custom Doer could return `(nil, nil)`) |
| 2 | pass | 0 | none (vet, staticcheck, errcheck, nilaway, exhaustive, go test -race all ok) |

Totals: 2 runs, first run failed. Problems by category: nilaway 1; compile 0, vet 0, staticcheck 0, errcheck 0, exhaustive 0, race 0, test-failure 0, panic 0, hang 0.
