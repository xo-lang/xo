# Build log: 10-log-pipeline (plain Go)

Command: `go build ./... && go vet ./... && go test -count=1 ./...`

| Run | Result | Distinct problems | Problems (category: description) |
|-----|--------|-------------------|----------------------------------|
| 1 | fail | 1 | test-failure: TestEveryValidLineCountedOnce expected service "web" to hold the last timestamp (20999), but line 19999 maps to service "db" (19999 % 3 == 1); wrong expectation in the test, not a code bug |
| 2 | pass | 0 | none |

Total runs: 2. First run: fail (1 test-failure).
