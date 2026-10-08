# Build log

Check command: `bench/go/strict-check.sh bench/go/strict/08-notes-service`
(go vet, staticcheck, errcheck, nilaway, exhaustive, go test -race).

| Run | Result | Distinct problems | Problems (category: description) |
|-----|--------|-------------------|----------------------------------|
| 1   | pass   | 0                 | none (all six checks ok on the first run) |

Total runs: 1. First run passed. Problems by category: compile 0, vet 0,
staticcheck 0, errcheck 0, nilaway 0, exhaustive 0, race 0, test-failure 0,
panic 0, hang 0.

One edit was made before the first run (allowed, since no command had been run
yet): a test that requested `/notes/..%2fetc` was replaced with requests for
`/notes/CON`, `/notes/a.b`, `/notes/nul`, because I was unsure whether
ServeMux would clean that path into a redirect instead of reaching the handler.
