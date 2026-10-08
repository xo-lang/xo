<!-- Prompt for arm xo, task 10-log-pipeline. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/xo-context.md}}

{{include bench/harness/prompts/common/protocol-xo.md}}

# Task

{{include bench/tasks/10-log-pipeline.md}}

# Required file layout

- `xo.mod` at the root: `module example.com/logpipeline`, then `edition 2026`.
- All `.xo` files in the root directory (one module). Tests go in files
  whose names end in `_test.xo`.
- `NOTES.md` at the root.

{{include bench/harness/prompts/common/output-contract.md}}
