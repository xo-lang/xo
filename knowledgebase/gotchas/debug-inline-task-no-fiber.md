# A running task need not be on a worker's running fiber

Status: Observed (2026-11-06, Xo Lang 1 found it, Xo Lang 2 fixed it, decision 0253)

## Symptom

`xo debug`'s `task N` on a task stopped at a breakpoint failed about once
in 240 sessions of cmd/xo TestDebugTasks on the linux/arm64 offload host:
`tasks` listed task 4 as running, but `task 4` answered "task 4 is
running: no stack of it can be read".

## Cause

The debugger found a running task's thread through the workers' running
fibers (W_RUNF of each record in P_WS), matching the fiber's F_TASK to
the task's record. Two other places run a task's code and were missed:
the root thread, which while main is parked runs queued fibers as a
worker does (std/native/llrt/pool.xo root_main) with the W_RUNF of its
own record, main's fiber's F_ROOT, which is not in P_WS; and a scope's
join, which may claim a task no worker took yet and run it on the
joiner's stack (tasks.xo run_claimed), on a fiber whose F_TASK is the
joiner's. Task 4 wakes from a 200 ms sleep, and which thread resumes it
is a race, so the test passed almost always (0 in 480 on a quiet host 1).

## Fix

compiler/debug tasks.xo: fiber_frames scans the root record's running
fiber as well as the workers' (and main's fiber from the deadlock record,
P_DLK DLK_MAINF), and each TaskRec carries its hosts (its enclosing
tasks, nearest first, then main): running_frames tries the task's own
fiber, then each host's, taking a host's thread only when its frames
hold run_claimed. Reading a task's state from runtime records: a running
task need not be on a worker in P_WS, nor own the fiber it runs on.
