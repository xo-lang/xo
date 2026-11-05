# A held ADR branch links its decision twice after a rebase

Observed 2026-11-05.

Symptom: pushq's fast repo checks fail TestDecisionNumbersUnique with
"NNNN-title.md is linked 2 times in decisions/README.md, want 1" for a
branch that rebased cleanly (git reported no conflict in the README).

Cause: the ADR claim flow commits a stub and its README line on main
first (CLAUDE.md "Decisions"). A branch written before that, or whose
README line was rewritten (Proposed to Accepted, a new summary), adds its
own line at a different place in the file, so the rebase merges both
lines without a conflict. Two held branches (0498, 0500) hit it on the
same day.

Fix: after rebasing a branch that carries an ADR, run
`grep -c "NNNN-" knowledgebase/decisions/README.md` and keep one line
(the branch's, which has the current status); or edit main's stub line in
place on the branch instead of adding a new one.
