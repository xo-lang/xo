# Build log

| Run | Result | Distinct problems | Problems (category: description) |
|-----|--------|-------------------|----------------------------------|
| 1 | fail | 1 | nilaway: http_test.go dereferenced `resp` from `http.Client.Post` after only checking `err` (reported at 2 sites, same nil source) |
| 2 | pass | 0 | none |

All other checks (vet, staticcheck, errcheck, exhaustive, `go test -race`) were ok on run 1.
