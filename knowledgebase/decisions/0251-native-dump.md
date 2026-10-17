# 0251: xo dump for native programs

Status: Accepted (lead, 2026-10-17); implemented the same day (xo-fa)

## Context

`xo dump <pid>` sends SIGQUIT and prints the dump the program writes:
its task tree with what each task waits on and where (core 11.4). Only
the Go backend's runtime wrote one; since native became the default
(decision 0248) a native program had none, though a task tree is what an
agent needs to understand a hang.

## Decision

- SIGQUIT's handler (poll.xo on_quit, installed with SIGINT's and
  SIGTERM's in entry.xo, not in test binaries) only counts the signal and
  writes a byte to the poller's self pipe, as SIGINT's does. The poller's
  thread, which every program has from its start, writes the dump
  (dump.xo, through cabi.xo dump_now). So a dump comes with one worker
  (XO_THREADS=1), from an idle server, and from a program whose tasks are
  all parked; the handler itself does nothing unsafe.
- The tree: from each root context (a registry ctx.xo keeps; the first
  is main's) down the cancellation nodes, which are already the scope and
  task tree: a task's node (its record freed by task_free), a scope's
  body (scope_free), and the scope's node of its tasks (shown through its
  children, as the Go backend's).
- A task's state: ready, finished, running, `in scope` (its context is in
  a scope it opened), `waiting for its tasks` (a scope body at its join),
  or `blocked in OP at FILE:LINE`. OP is the park reason the trace added
  (ctx.xo C_WHY, decision 0240), in the Go backend's words (receive,
  send, select, Mutex.with, await, sleep, serve, advance) and `network
  I/O` for a socket wait, which the Go backend's dump does not name; the
  reason now holds until the park ends. The location is the first frame
  of the program's own code: a parked fiber is unwound from its switch
  frame with the profiler's unwind table walk (unwind.xo fiber_walk); a
  wait no fiber holds (a root thread's, or a task run inline on one, as
  at a scope's join) follows the frame pointers from the park's caller,
  which every park records (ctx.xo C_PARK_FP).
- A task record keeps its context while it runs (tasks.xo T_CTX) and its
  spawn's location (T_LOC_P, T_LOC_N): `task N (spawned at main.xo:6)`,
  numbered as the Go backend numbers its threads (main is 1).
- Output: the Go backend's format (std/rt sched.go dumpText), on standard
  error and in `$XO_DUMP_DIR` (else $TMPDIR, else /tmp) as
  `xo-dump-<pid>.txt`; the program keeps running.
- Files and release builds: the same in every build. An earlier dump at
  the path is unlinked (a symbolic link itself, not its target), then the
  file is created with the profiler's rules (O_EXCL, O_NOFOLLOW, mode
  0600). Writing it in release builds too keeps `xo dump` working on the
  binaries agents debug; the dump holds names and locations, not values.
  (The lead suggested the file only with XO_DUMP_DIR in release builds;
  `xo dump` reads the file at the default path, so that would break it on
  release binaries.)

## Consequences

- cmd/xo TestDump runs on the Go backend, the native one, and the native
  one with XO_THREADS=1. Checked by hand: an idle HTTP server dumps with
  and without XO_THREADS=1.
- Differences from the Go backend's dump, follow ups: a scope's own site
  (`scope at main.xo:5`: native scopes do not record it), main's own wait
  at a scope's end (`blocked in scope end at main.xo:5 for task 2, task
  3`: native shows `main: in scope` and the body `waiting for its tasks`),
  a channel's or mutex's origin and holder (`on a mutex made at ...,
  held by task 2`). x86_64 fibers are not unwound yet (their waits show
  no line unless the frame pointer walk finds one).
- Cost: a store of the frame pointer and of the reason per park, two
  stores per task run (T_CTX), two per spawn (the location), 24 bytes per
  task and 8 per context.
- The walk is best effort, as the Go backend's: children are read under
  each node's lock; a task's state can change while it is read.

## Alternatives

- A dedicated writer thread waiting in sigwait: one more thread in every
  program for something rare; the poller is already there and already
  handles SIGINT and SIGTERM the same way.
- Writing from the signal handler: it would allocate and lock in a
  handler.
- Unwinding root threads by signaling them (the profiler's way): more
  machinery; the recorded frame pointer gives the line for the waits that
  matter (a task run inline, a join).
