# Build log: 06-job-queue (Xo 0.4, checker arm)

Command: `.tools/bin/xo-0.4 check bench/xo/v0.4/06-job-queue` (run 2 with `--json`).

| Run | Errors | Warnings | Diagnostics | Cause | Class |
|---|---|---|---|---|---|
| 1 | 0 | 0 | none | Complete first draft (queue.xo, worker.xo, api.xo, main.xo, queue_test.xo) checked clean. | n/a |
| 2 | 0 | 0 | none | Same code with `--json`, to confirm the diagnostics array is empty and the module name is picked up from xo.mod. | n/a |
| 3 | 1 | 0 | XO0401 | Deliberate probe: appended a temporary test with `let zz: Int = "x"` to confirm that `test` blocks in `_test.xo` files are type checked by `xo check` (they are). Probe removed right after. | probe, not a mistake |
| 4 | 1 | 0 | XO0701 | Deliberate probe: removed `uses clock` from `pub fn routes` to confirm effect checking reaches into closures registered with `r.handle` (it does, with a call chain note). Probe reverted right after. | probe, not a mistake |
| 5 | 0 | 0 | none | Final run on the restored solution. | n/a |

No code changes were needed between run 1 and run 5; runs 3 and 4 were
temporary probes of the checker's coverage, reverted from backups.
