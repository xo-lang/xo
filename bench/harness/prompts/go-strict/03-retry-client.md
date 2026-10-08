<!-- Prompt for arm go-strict, task 03-retry-client. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/protocol-go-strict.md}}

# Task

{{include bench/tasks/03-retry-client.md}}

# Required file layout

- `go.mod` at the root: `module benchtask` and `go 1.26`, no requirements.
- A library package is enough; no `main` is required.
- Tests in `_test.go` files.
- `NOTES.md` at the root.

{{include bench/harness/prompts/common/output-contract.md}}
