<!-- Prompt for arm go-strict, task 02-worker-pool. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/protocol-go-strict.md}}

# Task

{{include bench/tasks/02-worker-pool.md}}

# Required file layout

- `go.mod` at the root: `module benchtask` and `go 1.26`, no requirements.
- Any package layout under the root.
- Tests in `_test.go` files.
- `NOTES.md` at the root.

{{include bench/harness/prompts/common/output-contract.md}}
