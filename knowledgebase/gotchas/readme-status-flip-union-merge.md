# An ADR status flip in decisions/README.md doubles its line on a rebase

Status: Observed (2026-11-07, decisions 0545, 0547, 0551 and 0552).

## Symptom

After a rebase (pushq's, or `git rebase`), knowledgebase/decisions/README.md
holds the same ADR twice, once Proposed and once Accepted, and
TestDecisionNumbersUnique fails in pushq's fast checks ("is linked 2
times in decisions/README.md, want 1"). The other side's flip of a
neighbouring line can come back doubled too.

## Cause

.gitattributes merges the knowledgebase READMEs with `merge=union`, so
appended lines never conflict. A line changed on both sides (Proposed to
Accepted on one, an untouched or other edit on the other) is not an
append: union keeps both versions.

## Fix

After any rebase of a change that edits an existing README line (a
status flip), check `grep -c NNNN- knowledgebase/decisions/README.md` for
the ADRs near it and keep the line main has plus your own change, then
amend. Running TestDecisionNumbersUnique before pushq catches it.

Repaired automatically since 2026-11-07 (Xo Lang 1 hit it again on
0554): pushq passes the README at the push's merge base to adr-dedup.py,
which keeps, of a decision's two lines, the one that differs from the
base (the side that changed it). Two lines that both changed still stop
the push for a hand fix.
