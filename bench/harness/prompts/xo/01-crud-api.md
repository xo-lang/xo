<!-- Prompt for arm xo, task 01-crud-api. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/xo-context.md}}

{{include bench/harness/prompts/common/protocol-xo.md}}

# Task

{{include bench/tasks/01-crud-api.md}}

# Required file layout

- `xo.mod` at the root: `module example.com/crudapi`, then `edition 2026`.
- All `.xo` files in the root directory (one module). Tests go in files
  whose names end in `_test.xo`.
- `fn main` in the root module serves the API on `PORT`. The scorer
  builds the root and runs extra black box HTTP checks against it.
- `NOTES.md` at the root.

{{include bench/harness/prompts/common/output-contract.md}}
