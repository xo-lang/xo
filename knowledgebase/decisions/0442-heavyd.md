# 0442: heavyd, one daemon for the Mac's heavy slots, gates, measurements and quiet windows

Status: Accepted, switched (owner, 2026-10-31: "do 1", after the shadow
evening; agents/heavy.sh is the heavyd client, legacy-heavy.sh the bash
fallback). Earlier: accepted (owner, 2026-10-30: the design; the switch from shadow
mode and the per-class time limits wait for the shadow log review and the
owner's OK). Proposed by Xo Lang 2; the owner approved building it in Go
first, to be converted to Xo later.

## Context

`scripts/lead/heavy.sh` (584 lines of bash) schedules every Mac build,
test and measurement: two heavy slots (pushq, then gates, then bulk), a
gate pool with memory, count, load and stagger rules, measurements that
open their own quiet window and drain the machine, the memory reaper
(top's MEM, compressed pages counted), the gate stall detector, the gate
timeout and nested runs. Each caller is its own bash process that polls
lock directories under `$LEAD/locks` every 5 to 15 seconds, and each rule
is spread over every caller's copy of the loop:

- no one place knows the whole queue: `--status` prints the slots and two
  counts, not who waits, why or since when;
- a rule change is a change to every waiter's loop, and the rules interact
  (pass limits, urgent exemptions, gate yield, long run release); each new
  one so far came with a scenario in `test-heavy.sh` after an incident;
- slot runs and bulk runs have no wall clock limit (HEAVY_SLOT_TIMEOUT
  defaults to 0), so a hung bulk run holds a slot until someone notices;
- a gate logs why it waits (59357bca2); slot runs and measurements log
  only some of their reasons.

## Decision

1. **heavyd** (`scripts/lead/heavyd`, Go, standard library only, a
   `package main` of the repository module, gofmt, table tests) is one
   long lived daemon per Mac that owns the scheduling state: the slot
   queue, the gate pool, the measure queue and the quiet window. It
   listens on a unix socket, `$LEAD/heavyd.sock` (`XO_LEAD_DIR` as
   today).
2. **The client keeps heavy.sh's interface exactly.** `heavy.sh NAME
   CMD...` and `heavy.sh --status`, every `HEAVY_*` variable (HEAVY_CLASS
   gate|bulk|measure, HEAVY_DARWIN, HEAVY_PRIORITY, HEAVY_MAX_GB,
   HEAVY_QUIET, HEAVY_NESTED, the tuning variables) and `XO_HEAVY_SLOT`
   nesting mean what they mean now, read from the caller's environment
   and sent with the request. The exit codes stay: the command's own, 64
   for bulk without HEAVY_DARWIN=1, 75 for a refused measurement, 2 for
   usage. `NAME-cmd.log` keeps its `=== DATE start (...)`, `=== DATE exit
   RC`, `queued (gate)`, `gate waits:`, `nested in slot`, `stalled`,
   `refused: drain cap` and `no window yet` lines, which hangcheck.sh,
   pushq.sh, hourly.sh and test-heavy.sh parse.
3. **The client starts the job; the daemon decides.** The client (the
   thin heavy.sh, a bash process whose command line stays `heavy.sh NAME
   ...`, so hangcheck.sh's `ps` parsing keeps working) asks the daemon
   for admission and waits; on "start" it starts the command in a new
   process group with GOFLAGS, XO_SEPCOMP_BG=0 and XO_HEAVY_SLOT set as
   today, and output appended to the log, and reports the group's PID.
   The daemon watches the group read only (ps and top, as heavy_watch
   does) and, past a limit, tells the client to stop it; the client
   signals only the group it started, by the PID it captured (TERM, then
   KILL after the same grace as today). Neither side ever runs pkill,
   killall, or kill by name or pattern, and the daemon signals no
   process at all. This keeps the job in the caller's environment,
   working directory, user and file descriptors, with nothing to copy
   over the socket.
4. **Lock files stay as a mirror.** The daemon writes `locks/heavy*/pid`
   and `who` (`urgent`), `locks/gates/PID`, `locks/gatewait/PID`,
   `locks/queue/TICKET`, `locks/mqueue`, `locks/measure/pid`,
   `locks/quietwait/PID` and `$LEAD/quiet` and `quiet.owner` with the
   client's PID, as heavy.sh does, so hangcheck.sh, quiet.sh, diskcheck.sh
   and the lead's scripts run unchanged. `quiet.sh MINUTES` keeps working:
   the daemon reads `$LEAD/quiet` each tick as the lead's window. Holders
   the daemon did not admit (a heavy.sh from before the switch, or the
   fallback in 7) count as busy until their PID is gone; the daemon
   never signals them.
5. **New behavior.**
   - A wall clock limit for every class: gate HEAVY_GATE_TIMEOUT
     (default 90 min, as now), bulk HEAVY_BULK_TIMEOUT (default 360),
     other slot runs HEAVY_SLOT_TIMEOUT (default 180, was 0), measure
     HEAVY_MEASURE_TIMEOUT (default 120, was 0); 0 still means none.
     The defaults are proposed; the lead and owner set them at the
     switch.
   - `heavy.sh --status` keeps its first lines and adds a queue view:
     each running job (name, class, slot, PID, since, memory) and each
     waiter in admission order (name, class, since, and the reason it
     waits now).
   - Every waiter's log gets `=== DATE waits: REASON` when its reason
     changes (the gate's `gate waits:` line keeps its text), for slot
     runs and measurements too.
   - The daemon logs every decision to `$LEAD/heavyd.log`.
6. **The rules port as they are**, each with its dated comment: slot
   order (priority tickets, then ordinary, then bulk, first come first
   served across the slot and measure queues), bulk only in the last free
   slot while a non-bulk run waits, the second slot at HEAVY_MIN_FREE,
   the measurement pass rules (HEAVY_MEASURE_PASS_MIN, the two waiting
   measurements rule, the urgent slot exemption,
   HEAVY_MEASURE_LONGRUN_MIN), gates (HEAVY_GATE_MIN_FREE,
   HEAVY_GATE_MAX, HEAVY_GATE_MAX_LOAD with the HEAVY_GATE_LOAD_WAIT
   fallback, urgent gates skip the load ceiling and the window,
   HEAVY_GATE_STAGGER, HEAVY_GATE_YIELD_MIN), measurement windows
   (HEAVY_MEASURE_LEAD_RE and LEAD_WAIT, the drain to
   HEAVY_MEASURE_LOAD within HEAVY_MEASURE_DRAIN, HEAVY_MEASURE_ON_CAP,
   HEAVY_MEASURE_MIN, removal of a window only by the epoch its owner
   wrote, a stale window whose owner PID is gone), the memory reaper at
   HEAVY_MAX_GB over the whole tree and the gate stall detector
   (HEAVY_GATE_STALL_MIN, skipped while the tree holds a build.lock or
   fetch.lock). Nested runs (XO_HEAVY_SLOT set, HEAVY_NESTED not
   `queue`) never reach the daemon: the client runs them at once, as
   now.
7. **Fail safe.** When the socket does not answer, the client runs the
   current heavy.sh, kept as `heavy-legacy.sh`, so a dead daemon slows
   nothing down; the mirror in 4 keeps the two from overcommitting the
   slots. A client whose daemon connection drops mid run keeps its job
   running and only loses the watchdog until it reconnects; the daemon
   frees a slot when its client's PID is gone.
8. **Tests.** The scheduler is a pure function of its state, a clock and
   probes (free memory, load, process trees), so each of test-heavy.sh's
   19 scenarios becomes a Go table test on a fake clock that runs in
   milliseconds; scenarios 5 (hangcheck reads the mirror), 15 (nested),
   17 (stall, with real processes) and 18 (exit 64) also run end to end
   against a real daemon in a temporary XO_LEAD_DIR. Scenario 6 (bash -n
   of every lead script) stays in test-heavy.sh, which keeps testing
   heavy-legacy.sh until it is retired.
9. **Rollout.**
   1. Shadow mode: `heavyd -shadow` reads the lock files and the `NAME-cmd.log`
      lines heavy.sh writes, runs its scheduler on the same arrivals and
      logs, to `$LEAD/heavyd-shadow.log`, each decision it would make
      beside what heavy.sh did (who started, when, in which slot, and any
      stop). It writes no lock file and signals nothing. It runs beside
      heavy.sh for at least a day of normal load.
   2. The agent sends the lead the shadow log's disagreements and the
      switch plan. The lead installs (install.sh) and switches
      `agents/heavy.sh` to the client with the owner's OK. No agent
      replaces `~/xo-lead/agents/heavy.sh` itself.
   3. Later, a port of heavyd to Xo (the owner's "in Go first").

## Consequences

- One place holds the queue: `--status` answers who waits and why, and a
  new rule is one change in one process with a millisecond test.
- Slot and bulk runs gain wall clock limits; a job longer than its
  class's default needs the variable set (documented in agent-tools.md
  at the switch).
- A daemon is one more process to keep alive; the fallback in 7 means a
  dead one costs only the new features. hangcheck.sh gains a line for a
  socket that does not answer.
- The Go code is lead tooling, not product: it is not part of the fixed
  point and needs no seed.

## Alternatives

- **Keep extending heavy.sh.** Each rule is another loop in every
  waiter; the queue view and the wait reasons need the global state no
  waiter has.
- **The daemon starts the job** (the brief's first sketch). It would
  need the caller's environment, directory and descriptors copied over
  the socket, and a job would outlive a crashed daemon with no one to
  stop it; with the client starting it, every signal still comes from
  the process that started the group, and the job stays in its caller's
  context.
- **Write it in Xo now.** The owner chose Go first; the scheduler's pure
  core is meant to port directly.
- **launchd's resource limits.** They limit one process, not a queue
  with priorities, windows and pools.

## Addendum 2026-10-31: a gate's exit ends its stagger

The shadow review before the switch (2026-10-31) found five of its six
remaining holds were gates that had already exited while their
HEAVY_GATE_STAGGER still held the next gate. The stagger spaces gate
starts so two builds do not peak at once; a gate that has ended peaks no
more. heavyd's finish of the gate that started last now clears the
stagger and removes the gatestart lock it holds, and the shadow models the
same (a gate it would have started, no longer waiting or running, ends
its stagger). Approved by the owner with the switch; tests
TestFinishEndsGateStagger and TestShadowGateExitEndsStagger.

## Addendum 2026-10-31: the stall detector for every class

The owner, 2026-10-31: "too many runs get hung". The stall detector (no
CPU, no output and no new process) stopped only gates (HEAVY_GATE_STALL_MIN,
20); slot and bulk runs now get it too, after HEAVY_STALL_MIN minutes
(default 30, 0 turns it off), in the daemon and in the shadow. The
offload hosts' jobs get the same rule in offload.sh (OFF_IDLE_MIN, 15).
