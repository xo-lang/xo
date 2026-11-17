# 0702: an xw build system in Xo replaces the bash orchestration, stage by stage

Status: Accepted (owner, 2026-11-17, xw answer 61: "as written"; drafted by xw-1, task #248).

## Context

Agents build, test, gate and push through about 5,000 lines of bash:
heavy.sh (the heavyd client), gate.sh (named steps), offload-gate.sh (a
gate on a host), pushq.sh (the merge queue) and stdproven-offload.sh,
glued by environment variables and log files that xw (decision 0658)
reads back. Each script grew by incident; each rule is checked only by
the next incident. The lead's record of 2026-11-17 alone
(`~/xo-lead/notes/xw-1/build-system-evidence.md`):

1. A step named tests by pattern: gate.sh nohttp's `-run
   'Go|Replay|Holes'` matched all four TestDevGoldens* and ran about 6
   extra minutes in every gate, unseen; tsanmaps' `TestGolden/conc_.*`
   inside parentheses never ran the conc_ goldens at all (#249).
2. A latent failure: TestDevGoldensELF/native_sqlite_file failed for 7
   days, drawn only by a -short sample or a cold build, and then blocked
   every std/native and llrt push.
3. The merge lane: in-lane checks held each push about 3 minutes, about
   20 pushes an hour, 33 minutes median wait (#232); its first fix,
   "rerun when main touched a check's inputs", changed nothing because
   main gains 30 to 60 files during a wait (#254).
4. Placement by hand: Lang 1's gates waited 50+ minutes on host 4
   behind the lead's tier lease.
5. Jobs belong to sessions: closing azure-1's session would have stopped
   4 runs.
6. Environment and quoting leaks: a gate's OFF_MAX_MIN reached pushq's
   fast checks and failed every push (#169); scripted edits left `\&\&`
   in gate scripts; a wrong `-run` passes as "no tests to run"; bash 3.2
   read an apostrophe in `${x:+...}` as a quote and skipped a whole
   section of pushq (#254).
7. Agents spend tool calls on bookkeeping and polls: calls per commit
   rose from 29-30 to 44-46 after 2026-11-15, 2.4 to 2.9 of them status
   polls and 1.8 to 2.7 xw marks (#246; callmix).

Decision 0658 made xw the record of this work; it does not run the work.

## Decision

xw gains a build system written in Xo: jobs are values of defined kinds
with typed steps, explicit inputs and environment, and one record each;
xw places, runs, caches, retries and reports them. It replaces the bash
front ends stage by stage. Each stage lands behind the scripts it
replaces, runs beside them until its measure holds, and then the script
is retired.

### Stage 1: job ends are messages (removes case 7's polls; #247)

heavyd, offload-gate and pushq send each job's end to xw with its log and
owner (the task bound to the worktree, #246). The hook inbox shows the
owner's job ends on its next prompt or tool call (workstream 169faff).
A tiny `claude -p` (haiku, no tools, empty context: about 1.7K input
tokens) writes the at most 5 line verdict, and a second one whose only
tool is SendMessage (about 3.1K) delivers it to the owner's live
session, behind a switch (owner allowed, 2026-11-17). Then ccguard
retires polls, sleeps and `--wait` forms.
Measure: polls and waits per commit (callmix) to under 0.5.

### Stage 2: jobs are a graph of typed steps (removes cases 1 and 6)

A job kind is an Xo value: its steps, each with the command it runs as
argv (no shell), the tests it names (checked to exist when the kind
loads, as TestGateRunPatterns does for gate.sh today), the files and
environment it reads (an allow list: no inherited gate settings), the
host classes it may run on, and its time budget. A run is one record:
job, steps, inputs hash, host, times, result, log. Gate and push become
step definitions (stage 3), not scripts.
Measure: a step naming a missing test fails at load; zero environment
leak incidents.

### Stage 3: gate and push as definitions (removes cases 3 and 4)

gate.sh's steps and pushq's checks are step lists keyed by the paths a
change touches (gatelib's table, moved into Xo). xw picks the host by
lane load and data locality, not by the agent (heavyd lanes become xw
placement; heavyd stays the Mac's slot keeper). The merge lane runs
rebase and push only; every check runs before it on the rebased tree,
and in the lane only a check whose inputs both the push and the incoming
commits touch reruns (#254's rule, from recorded inputs instead of path
patterns).
Measure: lane hold under 30 s median; no gate waits over 10 min for a
host while another class has a free slot.

### Stage 4: flakes and failures are owned (removes case 2)

A failing step reruns once on another host. Pass on rerun: a flake
record (test, host, commit, signature); after N flakes in 7 days the test
is quarantined from gates (still run by the tier) with a task for its
owner. A failure on main is bisected by xw (mainbisect, #227) and filed
on the commit's author as a bug task with the failing test. The full
step set runs on each main commit (or each input hash) on an offload
host, so a latent failure surfaces the day it lands.
Measure: flakes handled without an agent's call; every main failure
attributed within an hour.

### Stage 5: results cached by input hash; scripts retired (removes case 5)

A step's result is keyed by the hash of its declared inputs (sources,
xo binary key, environment allow list, host class); a hit is reused
across agents and gates (Go's test cache and xo's backend cache stay
underneath, #251). Jobs belong to xw, not sessions: a session ending
leaves its runs going and their ends queued for the owner's next
session. Then heavy.sh, gate.sh, offload-gate.sh and pushq.sh become thin
callers of `xw run KIND ...`, and ccguard refuses direct runs of the
retired scripts.
Measure: gate minutes per push down by the cache hit rate; zero runs
lost to a session ending.

### Boundaries

- Only defined job kinds run: an agent asks for a kind with arguments,
  never an arbitrary script, which is the permission boundary the
  scripts lack today (a kind is added by a reviewed commit).
- Fast local path: a step under about 60 s on the Mac (a unit test, a
  single golden) runs directly in the agent's session through the same
  definitions, recorded but not queued.
- xw down: the CLI runs the job's steps locally with the same
  definitions and spools the record (decision 0658's spool), so xw is
  never the reason a push cannot happen; pushq keeps its lock as the
  last fallback until stage 5.
- heavyd keeps the Mac's slots and memory guard; xw decides what runs
  and where, heavyd when on the Mac.

## Consequences

- One place answers "what ran, where, on what inputs, why did it fail,
  whose is it", which today needs grep over five logs.
- Agents stop composing gate commands, choosing hosts and polling; per
  commit they ask for a kind and read one message.
- A new check is a step definition with declared inputs, testable before
  it runs anywhere; bash quoting and environment bugs leave with bash.
- Cost: the stages are several weeks of xw work; until stage 5 two paths
  exist (definitions and scripts), kept equal by running both and
  comparing results per stage.

## Alternatives

- Keep tuning the bash (the 2026-10 to 11 path): each fix holds until
  the next incident; #232 and #254 show a rule on paths is still a guess
  where recorded inputs would be exact.
- Buildkite, GitHub Actions self-hosted runners, or Bazel with remote
  execution: they bring queues, retries and caches, but the work is Xo's
  own (xo's backend cache keys, seeds, hosts that are containers on the
  Mac, heavyd's memory guard, the std evidence kinds), CI was retired
  (decision 0423) for its queue latency and cost, and an outside system
  splits the record xw keeps (0658) in two. Bazel would also mean
  describing the Xo build in Starlark. The owner's rule "remove, don't
  tune" (decision #58) favors one Xo program.
- Make xw only observe (0658 as is): it cannot fix placement, caching
  or flake handling, which need the runner.
