<!-- Prompt for arm xo, task 05-rate-limiter. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/xo-context.md}}

{{include bench/harness/prompts/common/protocol-xo.md}}

# Task

{{include bench/tasks/05-rate-limiter.md}}

# Required file layout

- `xo.mod` at the root: `module example.com/ratelimiter`, then `edition 2026`.
- All `.xo` files in the root directory (one module). Tests go in files
  whose names end in `_test.xo`.
- `NOTES.md` at the root.

{{include bench/harness/prompts/common/output-contract.md}}
