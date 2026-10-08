<!-- Prompt for arm go-plain, task 04-file-sync. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/protocol-go-plain.md}}

# Task

{{include bench/tasks/04-file-sync.md}}

# Required file layout

- `go.mod` at the root: `module benchtask` and `go 1.26`, no requirements.
- Any package layout under the root.
- Tests in `_test.go` files.
- `NOTES.md` at the root.

{{include bench/harness/prompts/common/output-contract.md}}
