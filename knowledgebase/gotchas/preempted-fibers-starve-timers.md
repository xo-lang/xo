# Preempted fibers kept timer wakes waiting

Status: Observed (2026-11-18, found by std/task's race stress under task #152).

Symptom: with more CPU bound tasks than workers, timers woke hundreds of ms late. std/evidence/task's
stress, `--race` on linux/arm64 host 3: `map_concurrent_until(500ms)` returned after 4522 ms and an
item sleeping 2 s finished Ok instead of TimedOut (all 8 items started within 1 ms). On the Mac with
`XO_THREADS=2` and 32 busy tasks a 20 ms sleep woke 550 to 840 ms late on arm64 and llvm; XO_RTSTATS
showed preempt=604 but inject=5.

Cause: a fiber preempted by SIGURG (decision 0318) goes on its worker's pin list, and pool.xo
`take_local` served next, the ring, then the pin list, all before find_work's inject queue. The poller
thread fires a due timer and pushes the woken fiber to the inject queue (it has no worker record), so
with every worker's pin list full the fiber waited for the fairness step, every 64th find_work round
of 10 ms slices. Same family as run-next-pair-starves-timers.md, through the pin list instead of the
run-next slot.

Fix: `take_pinned` after the inject queue and the poll in find_work (a preempted fiber is behind
everything queued, as a polled preempt's goes to the inject queue's tail). 20 ms sleeps now 0 to 36 ms
late; busy throughput unchanged. Test: golden busy_timers (XO_THREADS=2, median of five wakes under
200 ms; fails on both backends before).
