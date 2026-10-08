<!-- Prompt for arm xo, task 04-file-sync. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/xo-context.md}}

{{include bench/harness/prompts/common/protocol-xo.md}}

# Task

{{include bench/tasks/04-file-sync.md}}

# Required file layout

- `xo.mod` at the root: `module example.com/filesync`, then `edition 2026`.
- All `.xo` files in the root directory (one module). Tests go in files
  whose names end in `_test.xo`.
- `NOTES.md` at the root.

{{include bench/harness/prompts/common/output-contract.md}}
