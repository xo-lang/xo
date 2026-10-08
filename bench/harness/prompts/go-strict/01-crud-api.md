<!-- Prompt for arm go-strict, task 01-crud-api. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/protocol-go-strict.md}}

# Task

{{include bench/tasks/01-crud-api.md}}

# Required file layout

- `go.mod` at the root: `module benchtask` and `go 1.26`, no requirements.
- `package main` at the module root serves the API on `PORT`. The scorer
  builds the root and runs extra black box HTTP checks against it.
- Tests in `_test.go` files.
- `NOTES.md` at the root.

{{include bench/harness/prompts/common/output-contract.md}}
