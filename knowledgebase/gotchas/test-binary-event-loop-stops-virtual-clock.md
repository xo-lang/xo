# Starting the event loop in a test binary stops the virtual clock

Status: Observed 2026-11-18 (task #363).

## Symptom

Every test that sleeps or waits on a deadline under the virtual clock
(backoff, rate, native/llrt/netconn, pg, migrate, ui, uuid, native/time)
ran into `xo test`'s timeout ("the test did not finish within 1m0s of real
time"), its dump showing `main: blocked in sleep` or `blocked in advance`.
A test whose tasks all waited forever, which the runtime reports at once as
XO-F001, hung until the timeout instead.

## Cause

The first #363 change started the event loop (`poll_ev_start`) in every
test binary so the poller thread could answer SIGQUIT with the task tree.
With the loop started, the poller thread and the workers' polls (decision
0324) take part in the scheduler's deadlock check (pool.xo `dl_check`),
which the test world's clock move waits on (twrun.xo `kick`): the clock
never moved and no deadlock was ever found.

## Fix

A test binary's SIGQUIT gets its own self pipe and a plain thread that only
writes the dump (poll.xo `quit_start`, `quit_loop`; cabi.xo `quit_thread`),
never the event loop. A test that needs a hung (not deadlocked) program
keeps one task running, as cmd/xo/testdata/nogc/hangtask does. Before
starting runtime machinery in a test binary, run a module whose tests sleep
(`xo test --std-module backoff`).
