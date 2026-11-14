# publish: a hash quoted in Markdown translates by visit order

Status: Observed (Xo Lang 2, 2026-11-14, release 0.51.1)

## Symptom

`publish.sh prepare` for v0.51.1 refused: "main ... (published) is not an
ancestor of the new main: a published commit would change". No rule in
replacements.txt had changed. Only the last 3 published commits differed,
from the 2026-11-13 gotcha offload-gate-stopped-run-deletes-rerun.md: its
text quoted the private commit `d039e43ba` (0.51.0's release commit), which
the published copy kept as written and the new rewrite turned into its
public hash `44bd5ccd9`.

## Cause

The publish tools' file callback (fileinfo.py, decision 0249) maps a
commit hash quoted in a file to the commit's public hash through
filter-repo's map of commits it has rewritten so far. A commit not yet
rewritten when the file is visited stays as quoted. So the output depends
on the order filter-repo visits commits, and new refs change that order:
0.51.1's tag, branched from `d039e43ba`, brought that commit ahead of the
gotcha's commit.

## Fix

For 0.51.1: fileinfo.py keeps `d039e43ba` as quoted (a keep list with
this reason beside it), so the published bytes come out unchanged;
prepare then passed with fast-forward ok and purity 0 mismatches. To do:
translate the same way whatever the order (map only commits already
published or on main, or a second pass), with a test that runs the
translation in two visit orders and compares the bytes.

Avoid: quoting a release branch's commit hash in a file on main; name the
tag (v0.51.0) or the public hash instead.
