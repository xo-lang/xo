# A rebase of a changed decisions/README.md line can duplicate its neighbours

Observed 2026-10-30 (xo-lang-4). Status: Observed.

**Symptom.** pushq's fast checks failed TestDecisionNumbersUnique after
its rebase: "0439-test-result-cache.md is linked 2 times in
decisions/README.md, want 1", and the same for 0441, 0442 and 0443,
which the branch never touched. The README held both the old
(Proposed) and new (Accepted) lines of four decisions.

**Cause.** The branch changed one README line (0439 Proposed to
Accepted) whose neighbours main had changed the same way (0441 to 0443
accepted) since the branch's base. git's three-way merge matched the
hunks by context and kept both sides' versions of the block, with no
conflict reported.

**Fix.** Before pushing a README status change, take main's file and
apply the one line again: `git show origin/main:knowledgebase/decisions/README.md
> knowledgebase/decisions/README.md`, then the edit, then check `git diff
origin/main -- knowledgebase/decisions/README.md` shows one line
changed. TestDecisionNumbersUnique (run by pushq's fast checks) catches
what slips through.
