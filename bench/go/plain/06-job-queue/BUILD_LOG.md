# Build log

Command for every run (from this folder): `go build ./... && go vet ./... && go test -count=1 ./...`

| Run | Command | Result | Distinct problems | Problems |
|-----|---------|--------|-------------------|----------|
| 1 | `go build ./... && go vet ./... && go test -count=1 ./...` | fail | 1 | test-failure: TestHTTPAPI test helper tried to JSON decode the plain text "Method Not Allowed" body that ServeMux writes for a 405 (bug in the test helper, not the implementation) |
| 2 | `go build ./... && go vet ./... && go test -count=1 ./...` | pass | 0 | none |

Totals: 2 runs, first run failed. compile 0, vet 0, test-failure 1, panic 0, hang 0.
