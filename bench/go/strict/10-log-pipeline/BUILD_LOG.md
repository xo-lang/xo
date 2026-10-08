# Build log: 10-log-pipeline (strict Go)

Note: before the first run (no command had been executed yet) two test logic bugs were fixed on re read: a wait loop in the backpressure test that could hang, and a queue size of 8 in the shutdown test that would deadlock against workers gated on 50 accepted lines.

| Run | Result | Distinct problems | Problems (category: description) |
|-----|--------|-------------------|----------------------------------|
| 1 | fail | 6 | errcheck: main.go:33 unchecked `fmt.Fprintln(stderr, ...)` (--out required); errcheck: main.go:37 unchecked `fmt.Fprintln(stderr, ...)` (--workers); errcheck: main.go:41 unchecked `fmt.Fprintln(stderr, ...)` (--flush); errcheck: main.go:45 unchecked `fmt.Fprintf(stderr, ...)` (mkdir error); errcheck: main.go:61 unchecked `fmt.Fprintf(stderr, ...)` (run error); test-failure: TestEveryValidLineCountedOnce expected wrong service for the last timestamp (test bug: line n-1 belongs to db, not web) |
| 2 | pass | 0 | all checks ok (vet, staticcheck, errcheck, nilaway, exhaustive, test -race) |
