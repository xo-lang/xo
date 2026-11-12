# Browser memory of the playground pages (decision 0615)

One row per run of scripts/browser-memory.sh --record: the /repl/ compiler's
peak over the 30-entry session and the /play/ tab's footprint peak over 30
builds and runs, in headless Chrome on the development Mac, against the budgets in the
website's data/memory.json. A release's row (vX.Y.Z) is committed in its
version commit; scripts/release-publish.sh refuses a tag without an ok row.

| label | date | xo commit | /repl/ compiler peak (budget) | /play/ tab footprint peak (budget) | result |
|---|---|---|---|---|---|
