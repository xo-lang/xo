# Build log: 03-retry-client (Xo 0.4, with checker)

The full solution (retry.xo, profile.xo, retry_test.xo, xo.mod) was written
before the first command was run.

| Run | Errors | Warnings | Diagnostics | Cause | Class |
|---|---|---|---|---|---|
| 1 | 0 | 0 | none | First check of the complete solution passed. | n/a |
| 2 | 0 | 0 | none | Same check with `--json` to confirm one module (`bench/retry_client`) and an empty diagnostics list. | n/a |
| 3 | 3 | 0 | XO0110 (x2), XO0401 | Deliberate probe: two temporary lines (`let unused_probe = 1`, `let probe_r: Int = "x"`) added to a test block in `retry_test.xo` to confirm test files are actually type checked. File restored from a copy right after the run. | n/a (probe, not a mistake) |
| 4 | 0 | 0 | none | Check after restoring the test file. Final status. | n/a |

Final: 0 errors, 0 warnings after 4 runs (1 real check, 1 JSON confirmation,
1 probe, 1 confirmation after the probe). No code changes were needed to
satisfy the checker.
