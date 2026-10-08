<!-- Prompt for arm xo, task 08-notes-service. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/xo-context.md}}

{{include bench/harness/prompts/common/protocol-xo.md}}

# Task

{{include bench/tasks/08-notes-service.md}}

# Required file layout

- `xo.mod` at the root: `module example.com/notes`, then `edition 2026`.
- One subdirectory per module: `domain/`, `store/`, `api/`, and `server/`
  (which holds `fn main`). A module imports a sibling with
  `use example.com/notes/<dir>`. Add more directories if you need them
  (for example for the dependency rule test).
- Tests go in files whose names end in `_test.xo`.
- `NOTES.md` at the root.

{{include bench/harness/prompts/common/output-contract.md}}
