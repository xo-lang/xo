# np_break skips a held poll token: the root slept with work pending

Status: Observed (2026-10-25)

Symptom: a program (golden borrow_callbacks, 25 hangs in 36000 runs under
a load of 25, also on the 0.29.0 and 0.30.0 seeds) hung with the root thread
in kevent, every worker in go_idle's futex wait and no work running.

Cause: np_block checks for work, then np_poll takes the poll token (FREE to
BLOCKED). np_break (after a push, or main's end) pokes only when the token
reads BLOCKED or FREE. While another worker held the token for a round
(WORKER), a push in that window poked nothing, the holder released the token,
and the root took it and waited with no limit although work or main's end had
been published.

Fix: poll.np_poll rechecks pool.np_pending (work in the pool, or main ended)
right after the token reads BLOCKED, so a later push pokes and an earlier one
is seen by the check. A/B at load 25, 36000 runs each: 25 hangs before, 0
after (0 of 72000 in two runs).

Lesson: a "look for work, then block" path needs the check after the state
change that makes wakers act, not only before it.
