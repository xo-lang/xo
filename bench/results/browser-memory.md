# Browser memory of the playground pages (decision 0615)

One row per run of scripts/browser-memory.sh --record: the /repl/ compiler's
peak over the 30-entry session and the /play/ tab's footprint peak over 30
builds and runs, in headless Chrome on the development Mac, against the budgets in the
website's data/memory.json. A release's row (vX.Y.Z) is committed in its
version commit; scripts/release-publish.sh refuses a tag without an ok row.

Run to run variance (2026-11-12, 0.50.0 and 0.50.1 assets on the same Mac):
the /repl/ peak is 37 MB every run; the /play/ tab's footprint peak was 447,
449, 472, 480, 482 and 501 MB (a spread of about 12%, mostly how far
TurboFan's background compile of the compiler module overlaps the runs),
against a budget of 554 (the 482 baseline + 15%). A single run over the
budget is rerun once before it counts; over again, it is a regression
(a raised baseline is a deliberate commit, decision 0615).

| label | date | xo commit | /repl/ compiler peak (budget) | /play/ tab footprint peak (budget) | result |
|---|---|---|---|---|---|
| v0.50.0 | 2026-11-12 | - | 37 MB (43) | 447 MB (554) | ok |
| v0.50.1 | 2026-11-12 | - | 37 MB (43) | 501 MB (554) | ok |
| v0.51.0 | 2026-11-14 | - | 37 MB (43) | 407 MB (554) | ok |
