# Planted bug benchmark

For each task, single bugs are planted into a copy of the plain Go solution.
Each variant is then run through the plain loop (`go build`, `go vet`,
`go test`) and the strict loop (`strict-check.sh`). Later, the same bug
classes are planted in Xo solutions to check whether Xo rejects them at
compile time.

Layout per task: `planted/<task>/base/` (copy of `go/plain/<task>`),
`planted/<task>/variants/<bug-id>/` (only the changed files),
`planted/<task>/manifest.json`.

Bug classes:

| Id | Class | Xo prevents? |
|---|---|---|
| X1 | ignored error | yes: unhandled `Result` is XO0201 |
| X2 | nil dereference or nil map write | yes: no nil |
| X3 | missing case in a switch over a closed set | yes: exhaustive `match` (XO0301) |
| X4 | data race on shared state | yes: no shared mutable values |
| X5 | goroutine leak | yes: tasks cannot outlive their scope |
| X6 | lost update (copy modified, never written back) | partly: XO0520 |
| N1 | off by one | no |
| N2 | wrong HTTP status or error kind | no |
| N3 | wrong time, backoff, or limit arithmetic | no |
| N4 | inverted or wrong condition | no |

Bugs are planted only in non test code, so detection measures the existing
tests and tools. `run_detection.py` produces `results.jsonl`. A strict tool counts as catching a bug only if it reports a finding the unmodified base does not.
