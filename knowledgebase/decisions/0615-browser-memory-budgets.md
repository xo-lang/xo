# 0615: Memory budgets for the playground pages, checked in every release

Status: Accepted (owner, 2026-11-12, relayed by the lead; first used for 0.50.0)

## Context

The playground's REPL compiler grew to 49 MB in 0.49.1 before anyone
measured it, and the owner set a bar of 50 MB (decision 0583, brought
down to 37 MB by c99afd99f and 40e055f29). The /play/ tab (REPL never
opened) holds about 1.0 to 1.4 GB of renderer memory over 30 runs. No
release step measured either, so both could grow unnoticed. Owner,
2026-11-12: every release must check /repl/ and /play/ memory so they
cannot quietly grow.

## Decision

- The website repository holds the test and its budgets:
  `scripts/test-memory.mjs` drives headless Chrome over the built site,
  and `data/memory.json` holds the budgets, the 30-entry REPL session and
  each entry's expected value.
  - /repl/: the REPL compiler's wasm memory peak over the session. It
    fails above 50 MB (the owner's bar) or above the 37 MB baseline plus
    15%, whichever is lower (43 MB), or on a wrong value.
  - /play/: the renderer processes' RSS peak (ps, the page and its
    workers) over 30 runs of the default program. It fails above the
    baseline plus 15%. The baseline is 1376 MB: the highest of 3 runs on
    0.50.0's assets (1374, 1376 and 1307 MB, headless Chrome on the
    development Mac), so the budget is 1582 MB.
- `scripts/browser-memory.sh [--record LABEL] SITE_REPO` in this
  repository builds the assets from the tree's bin/xo in a scratch
  worktree of the site and runs the test. It appends a row (label, date,
  commit, both peaks and budgets, ok or FAIL) to
  bench/results/browser-memory.md, or to $BROWSER_MEMORY_LOG.
- Release: the version commit records its row (`--record vX.Y.Z`), and
  `scripts/release-publish.sh` refuses a tag whose tree has no ok row for
  it. A release cannot be published while the test fails.
- The lead's 8-hourly tier (scripts/lead/tier.sh) runs it on the Mac
  (HEAVY_CLASS=bulk HEAVY_DARWIN=1), with rows in
  ~/xo-lead/notes/lead/browser-memory.md, so a growth shows within hours.
- Raising a budget or a baseline is a deliberate commit to the site's
  data/memory.json that says why. Nothing raises one automatically.

## Consequences

- About 6 minutes per run (the asset build, two fresh Chrome profiles).
- RSS depends on the Chrome version and the machine. A Chrome update that
  moves /play/ past its budget fails the tier. The fix is to check whether
  the growth is ours (rerun on the previous release's assets) and, if it
  is not, to commit a new baseline with that reason.
- The REPL needs JSPI (Chrome 137 or later). An older Chrome fails as
  "could not run" (exit 2), not as a budget failure.

## Alternatives

- performance.measureUserAgentSpecificMemory for /play/: it needs
  cross-origin isolation, which the page does not have, and it leaves out
  V8's compiled code, which is a large part of the tab.
- A check only at release time: a growth would show days after the commit
  that caused it. The tier step finds it within 8 hours.
- Automatic baselines from the last release: every run could then grow by
  15% without anyone deciding it.
