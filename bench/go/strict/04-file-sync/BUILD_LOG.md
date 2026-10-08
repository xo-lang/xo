# Build log

| Run | Result | Distinct problems | Problems |
|-----|--------|-------------------|----------|
| 1 | fail | 1 | nilaway: sync.go:399 "unassigned variable `entries` sliced into" for `entries[i]` inside a reverse index loop bounded by `len(entries)` (judged a false positive, fixed by iterating with `slices.Backward`) |
| 2 | pass | 0 | all checks ok (vet, staticcheck, errcheck, nilaway, exhaustive, test -race) |
