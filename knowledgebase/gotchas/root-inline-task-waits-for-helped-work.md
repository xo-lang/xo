# A task the root thread runs inline waits for the work it helped with

Status: Observed (2026-10-20, `xo lsp`, decision 0284)

Symptom: a native server whose loop is main's body, or a task main's
body awaits, answers late while another task computes. `xo lsp` with a
check in a worker task left formatting requests unanswered for up to
7.3 s during the startup checks (the replies came in a burst when a
check ended), with workers idle. `xo trace` showed the loop's wake
handed off on one worker and the loop run only later, on worker 1 (the
root thread), right after the check there ended. Runnable latency max
3.2 s.

Repro (2026-10-20, arm64 and LLVM backends alike, 12 CPUs): a loop task
receives a tick every 20 ms from a ticker task while a worker task
computes for about 100 ms per job with no runtime calls. With
`lp.await` in main's body, 17 to 19 of about 20 ticks arrived more than
10 ms late (worst about 110 ms, a job's length); with main's body
waiting on a channel the loop sends to at its end, none did (4 runs
each, back to back).

Cause: the root thread is not a fiber. Main's body runs on it, and
`scope_join` (pool.xo) runs the scope's queued tasks inline on its
stack (`run_claimed`), also without a fiber. When such an inline task
parks, `park_wait` (no fiber, `rw` the root's record) runs other queued
work on the same stack while it waits: here the check, run as a fiber
by `run_item`. The parked task cannot move to another worker, and the
root returns to it only when that work parks or ends, so its wake (a
message, a tick) waits for the whole check. Tasks that `run_item`
starts get fibers, and a woken fiber goes to the waker's queue, so any
worker runs it.

Fix (the program's side, compiler/cli/servers/lsp.xo run_lsp): run the
loop in a spawned task and have main's body wait for it on a channel
(`fin.recv()`), not `await`: a root that waits on a channel runs other
tasks through `run_item`, on fibers. A runtime fix would keep a parked
inline task's thread from running long work, or give inline tasks
fibers; open, for decision 0077's helping rules.
