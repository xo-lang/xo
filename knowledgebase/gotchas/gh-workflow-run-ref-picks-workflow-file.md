# `gh workflow run` without --ref runs main's workflow file

Status: Observed (2026-10-14, runs 38633067906 and 38633071922).

Symptom: a CI run of a branch that changes `.github/workflows/ci.yml`
and `scripts/ci/suite.sh` together fails in the plan job with a step
the branch removed (`unknown suite xcross`).

Cause: `gh workflow run ci.yml -f ref=BRANCH` takes the workflow file
from the default branch (main); the `ref` input only chooses what the
jobs check out. Main's ci.yml then ran the branch's suite.sh.

Fix: when a branch changes ci.yml, dispatch it from the branch too:
`gh workflow run ci.yml --ref BRANCH -f ref=BRANCH -f suite=...`.
Branches that leave ci.yml alone need no `--ref`.
