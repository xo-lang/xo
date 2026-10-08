# Build log: 01-crud-api (Xo 0.4, with checker)

Checker: `.tools/bin/xo-0.4 check bench/xo/v0.4/01-crud-api`

The complete solution (store.xo, api.xo, main.xo, api_test.xo, xo.mod) was
written before the first run.

| Run | Errors | Warnings | Diagnostic codes | Cause | Class |
|---|---|---|---|---|---|
| 1 | 0 | 0 | none | Text output: `1 module(s), 0 error(s), 0 warning(s)` | n/a |
| 2 | 0 | 0 | none | Same check with `--json` to confirm: `"diagnostics": []` | n/a |

Total runs: 2 (one real, one JSON confirmation). No code was changed between
runs.

Caveat: a clean check only shows the program type checks. The compiler cannot
run programs yet, so the tests have not been executed and runtime behavior
(JSON shape, status codes, Patch decoding) is unverified.
