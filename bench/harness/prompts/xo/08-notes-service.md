<!-- Prompt for arm xo, task 08-notes-service. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/xo-context.md}}

{{include bench/harness/prompts/common/protocol-xo.md}}

# Task

{{include bench/tasks/08-notes-service.md}}

# Required file layout

- `xo.mod` at the root: `module example.com/notes`, then `edition 2026`.
- One subdirectory per module: `domain/`, `store/`, `api/`, and `server/`
  (which holds `fn main`). A module imports a sibling with
  `use example.com/notes/<dir>`. Add more directories if you need them.
- The dependency rule: Xo states import rules as `deny` lines in xo.mod
  (core 4.2, "Import direction rules"); `xo check` and `xo test` fail at
  a forbidden `use` (XO1312). That is how this task's "a test must fail"
  is met in Xo.
- Tests go in files whose names end in `_test.xo`.
- `NOTES.md` at the root.

{{include bench/harness/prompts/common/output-contract.md}}
