<!-- Prompt for arm xo, task 03-retry-client. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/xo-context.md}}

{{include bench/harness/prompts/common/protocol-xo.md}}

# Task

{{include bench/tasks/03-retry-client.md}}

# Required file layout

- `xo.mod` at the root: `module example.com/retryclient`, then `edition preview`.
- All `.xo` files in the root directory (one module). Tests go in files
  whose names end in `_test.xo`.
- A library: no `fn main` is required.
- `NOTES.md` at the root.

{{include bench/harness/prompts/common/output-contract.md}}
