# Build log: 02-worker-pool (Xo 0.4, with checker)

Command: `.tools/bin/xo-0.4 check bench/xo/v0.4/02-worker-pool` (plus `--json` where noted).

The full solution (`xo.mod`, `pool.xo`, `main.xo`, `pool_test.xo`) was written before the first run.
After writing it and before run 1, I made one edit without running anything: I added the
missing `use std/log` to `pool.xo` and dropped a misleading log field.

| Run | Errors | Warnings | Diagnostic codes | Cause | Class |
|---|---|---|---|---|---|
| 1 | 0 | 0 | none | First check of the complete solution | n/a |
| 2 | 0 | 0 | none | Same code with `--json`, to confirm the module list and that there were no diagnostics | n/a |
| 3 | 1 | 0 | XO0110 | Deliberate probe: I added `let probe_unused = 1` to a test block to confirm that `_test.xo` files and test blocks are actually checked. The probe was reverted right after the run. | probe, not a mistake |
| 4 | 0 | 0 | none | Final check on the restored files | n/a |

Final status: 0 errors, 0 warnings.
