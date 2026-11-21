# 0736: Workstream policy as data: validated, simulated, recorded, reversible

Status: Accepted (owner, xw decision #246, 2026-11-21; written by xw-1,
task #598). Part of
[0732](0732-one-scheduler.md), and step 5 of its migration. Scope: xw
decision #243 item 2 (strong primitives, policy as data).

## Context

Every rule lives in a script or a config file edited by hand:

- heavyd's limits in config.go and its environment variables;
- the lanes in heavyd-lanes.conf;
- push rules in pushq.sh;
- alert rules in alerts.xo;
- schedules in xw-schedule.conf.

A change is either a commit and an install, or an edit to a live file.
Neither is checked before it takes effect or simulated against recent
history. Who changed what, and why, is spread over commits and notes.
For example, the CAP 1 rule for host 1 is recorded only in a comment in
heavyd-lanes.conf.

## Decision

### What is policy

These are rows of a typed kind:

- `machine` (0733);
- `job_type` (needs, shards, detail globs, stall policy);
- `batch`;
- `placement` (flags such as no:llvm and timing);
- `admission` (reserves, the core factor, the per agent cap);
- `alert_rule` (an event query, a condition, a clear time, a target
  role);
- `push_policy` (required evidence per path pattern, the merge lane,
  squash rules);
- `schedule`;
- `role` (which agent gets which task kinds).

Every kind has a JSON schema in the core. A plugin may add fields under
its own namespace (`xo.*`), never in the core's fields.

### How a change is made

Every policy change goes through `xw policy` (the only writer):

1. `xw policy edit KIND NAME` (or `set` from a file) writes a draft.
2. **Validation.** The draft is checked against the schema, and against
   cross-row checks: a job type's kind exists on some machine; an alert
   rule's event kind has been recorded; no cycle in a batch's `after`.
3. **Simulation.** `xw policy simulate DRAFT` replays the last 7 days of
   jobs and events with the draft in place. It prints what would have
   changed: jobs placed elsewhere, waits longer or shorter, alerts that
   would have fired or not, pushes refused or allowed, and the predicted
   wall time of the batches that ran.
4. **Applying.** `xw policy apply DRAFT --why TEXT` records who applied
   it, why, the simulation's summary and the previous version, then
   makes it live. `--watch` applies it in watch-only mode: the scheduler
   or alert engine logs what the new rule would decide beside what the
   live rule decides, for the given time, and then the change is
   applied or dropped. Only one draft per row is in watch-only mode at a
   time.
5. **Rollback.** `xw policy rollback ID` restores the previous version
   of every row that change touched, records the rollback, and runs the
   smoke check for that kind.
6. `xw policy log [KIND] [NAME]` shows the history.

An owner decision (`xw decide`) that sets a rule names the policy change
that carries it out, and the change names the decision, so the two link
both ways.

### Alerts as rules

alerts.xo's built-in rules become `alert_rule` rows over the events
table, as #595's mac-overload is written today:

- a query;
- a condition;
- a key;
- a clear time;
- a target.

The core keeps the engine and the rules that depend only on the core
(idle, unacked, waits-for-fit, stall, machine-down). The Xo plugin holds
the rest.

## Consequences

- A rule change is checked against a week of history before it takes
  effect, and undone with one command.
- The first move of today's rules into rows is a large one-time job. It
  is done per kind at the matching migration step of 0732, with each
  script removed in the same change (#245).
- Simulation is only as good as the events recorded. A rule over data
  that xw does not record cannot be simulated, and says so.

## Alternatives

- **Policy files in the repository, reviewed as commits.** This gives
  history, but no simulation, no watch-only mode, and a deploy for every
  change.
- **Policy as code in the plugin** (Xo functions). It is more
  expressive, but it cannot be validated or simulated without running
  the code. Kept for result parsers only.
