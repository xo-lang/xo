# A rerun of a timed out GitHub Pages deploy is cancelled at once

Status: Observed (2026-11-05, Xo Lang 2, publishing /releases/0.40/).

## Symptom

xolang.dev's pages.yml run for 44fb7da21 built, then its deploy job sat in
`updating_pages` for 10 minutes and failed with "Timeout reached, aborting!"
(the action cancels its deployment). `gh run rerun RUN --failed` ran the
deploy job again and it failed in seconds: "Created deployment for
44fb7da21... ##[error]Deployment cancelled." The site kept the previous
build (/releases/0.40/ answered 404).

## Cause

A rerun reuses the run's artifact and creates the Pages deployment under the
same ID (the commit, `pages_build_version`), which the first attempt had
already cancelled, so Pages cancels it again.

## Fix

Start a fresh run: `gh workflow run pages.yml -R xo-lang/xolang.dev --ref
main` (the workflow has `workflow_dispatch`), then watch it with `gh run
watch RUN --exit-status --interval 10` and check the live URL.
