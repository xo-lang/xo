<!-- Prompt for arm go-strict, task 08-notes-service. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/protocol-go-strict.md}}

# Task

{{include bench/tasks/08-notes-service.md}}

# Required file layout

- `go.mod` at the root: `module benchtask` and `go 1.26`, no requirements.
- Packages in subdirectories: `domain/`, `store/`, `api/`, and `server/`
  (`package main`). Add more if you need them (for example for the
  dependency rule test).
- Tests in `_test.go` files.
- `NOTES.md` at the root.

{{include bench/harness/prompts/common/output-contract.md}}
