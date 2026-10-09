# Parallel agents pick the same ADR number

Status: Observed (2026-10-08 to 2026-10-09, at least eight times)

Symptom: two branches each add `knowledgebase/decisions/NNNN-*.md` with
the same NNNN (0044, 0051, 0058, 0060, 0070, 0095, 0097 collided), and
the merge has two decisions under one number; diagnostic codes collided
the same way once (XO1309).

Cause: each agent works in its own worktree and takes "the next free
number" from the main checkout as it was when the agent started.

Fix: at merge, renumber the later branch's ADR to the next free number
(`git mv`, the `# NNNN:` title, the README entry, and every "decision
NNNN" reference that branch added; leave references to the other ADR
alone), and do the same for a colliding XO code. Prevention when
starting parallel agents: assign each agent its ADR number (and XO code
range) in its prompt.
