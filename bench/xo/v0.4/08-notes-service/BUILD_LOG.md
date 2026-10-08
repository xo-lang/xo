# Build log: 08-notes-service (Xo 0.4, with checker)

Command: `xo-0.4 check ~/godev/claude-space/codelang/bench/xo/v0.4/08-notes-service`

| Run | Errors | Warnings | Diagnostics |
|---|---|---|---|
| 1 | 4 | 0 | `XO0601` x3: `module 'testing' not found` at `testing.TestOs` in helper function signatures of `api/api_test.xo` and `store/store_test.xo`. Category (c) suspected checker bug, see NOTES.md: core 9.2 and std 8 say `testing` is predeclared anywhere in a `_test.xo` file. Worked around by adding `use std/testing`. `XO0405` x1: `cannot infer the type of parameter 'p'` in `os.fs.files().keys().filter(fn(p) { ... })` in `store/store_test.xo`. Cascade of the XO0601 above (the type of `os` was unresolved, so `files()` had no type); (c), disappeared once `testing` resolved. |
| 2 | 0 | 0 | none |

Totals: 2 runs, 4 errors on the first run (3 x XO0601, 1 x XO0405), 0 errors at the end.

Before run 1, while still writing (no command had been run), I removed a
`testing.run_main(main, os)` test from `server/main.xo` because `serve()`
never returns in a test without cancellation, so it would hang. That was a
design correction, not a diagnostic fix.
