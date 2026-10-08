# Build log: 09-inventory (strict Go)

| Run | Result | Distinct problems | Problems (category: description) |
|-----|--------|-------------------|----------------------------------|
| 1 | fail | 3 | nilaway: inventory_test.go:197 `resp.StatusCode` after `http.Post` flagged as possible nil deref (err was checked); nilaway: inventory_test.go:357/360 `resp.Body` after `http.DefaultClient.Do` flagged as possible nil deref (err was checked, t.Fatal on error); nilaway: inventory_test.go:365 `closeBody` param `resp.Body` reached from `http.Post` result |
| 2 | pass | 0 | all checks ok (vet, staticcheck, errcheck, nilaway, exhaustive, test -race) |
