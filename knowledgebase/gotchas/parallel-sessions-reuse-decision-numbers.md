# Parallel sessions reuse decision numbers

Status: Observed (2026-10-10)

Symptom: two decisions numbered 0132 (mascot, P-256 signer) and two
numbered 0133 (domains, real-time functions) on main the same day, and
two decisions (0111, 0112) linked twice in `decisions/README.md`.

Cause: sessions working at the same time (often in worktrees) each took
"the next free number" from their own checkout, then merged into main.

Fix: renumber the decision with fewer references to the next free number,
update every reference (`grep -rn NNNN --exclude-dir=runs`; beware
unrelated hex constants such as `0x0132`), and add a "Numbered NNNN
until" line so old commit messages stay traceable. Done 2026-10-10:
P-256 0132 to 0134, domains 0133 to 0135.
`TestDecisionNumbersUnique` (`internal/diag`, part of `go test -short`)
now fails on a duplicate number or an index line missing or repeated.
Before merging a branch that adds a decision, `ls knowledgebase/decisions`
on main and renumber if the number was taken meanwhile.
