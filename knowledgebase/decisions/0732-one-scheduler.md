# 0732: Workstream's target architecture: one control plane that owns every machine

Status: Proposed (xw-1, task #598, 2026-11-21). Design only: the owner
decides before any code, and code starts after 0.55.0 (xw decision #243).
Scope set by the owner through the lead on 2026-11-21: one scheduler
that owns every machine (#598), one that plans as well as admits (xw
decision #242), Workstream as a generic core with Xo as its first plugin,
policy kept as data, Workstream as the control plane (#243), SQLite in
place of Postgres with no part of the control plane in Docker (#244),
and migration done in place with no parallel run (#245).

Module ADRs: [0733](0733-workstream-planner.md) (the planning
scheduler), [0734](0734-workstream-sqlite.md) (storage),
[0735](0735-workstream-control-plane.md) (hosts, runs, logs, outages),
[0736](0736-workstream-policy-data.md) (policy as data). When 0733 and
0735 land, they supersede the admission parts of 0442 (heavyd) and 0451
(heavyd lanes).

## Context

### Seven schedulers, and none of them sees the whole machine

| Scheduler | What it admits | Resource model it assumes |
|---|---|---|
| heavyd Mac classes (0442; scripts/lead/heavyd/config.go) | gate, bulk, measure and quick runs on the Mac | the Mac alone: 3 heavy slots; a gate starts at 35% free memory, with fewer than 5 gates and load under 48, 20 s apart; a job's tree is stopped past HEAVY_MAX_GB (32 by default) |
| heavyd lanes (0451; ~/xo-lead/heavyd-lanes.conf) | offload.sh, offload-gate.sh, amd64-run.sh, pushq's merge, docker runs | slots per host in arrival order: arm64 (host 1) 3, pass2 3, pass3 3, pass4 2, amd64 6, win 2, winarm 1, merge 1, docker 1; a slot has no size, and a memory lease takes every slot |
| offload-gate.sh routing | which host a gate takes | memory gates go to host 4 first; short gates go to host 4 when hosts 2 and 3 are held whole; lease minutes come from gate history (#585) |
| tier.sh, tier-mac.sh | the 8 hour tier | its own floors: 50 GB free disk to start; TIER_MAX_GB 40 per step (the 64 GB cap in tiers 919 and 920); Mac steps run together, then one at a time after a memory stop; a fixed split of steps between the Mac, host 2, amd64 and Windows |
| ctier.sh | the continuous tier steps | one lease per step, sized from a floor or 1.3 times the slowest run; llvm runs as 4 leases |
| pushq.sh | pushes | one merge lease; its fast checks run as Mac gate jobs |
| xw schedule (xw-schedule.conf) | 11 recurring jobs | time only: a job starts when it is due, whatever the machines are doing |

Hosts 3 and 4 are Docker containers on the Mac. The Docker VM has 20 of
the Mac's 30 cores and 57328 MiB of its 96 GB (Docker Desktop
settings-store.json, read 2026-11-21). Host 3 reports 20 CPUs (offload
gate line, 2026-11-21). Host 4 is capped by its cgroup at 8 CPUs and
24 GB (offload.md).

heavyd's Mac rules see the VM only as used memory and load. The lanes
treat hosts 3 and 4 as separate machines with their own slots. So the
same 96 GB is admitted three times over: Mac gates by free percentage,
a tier's Mac steps by their own cap, and up to 5 slots of container
work.

xw itself stores its data in Postgres in a Docker container
(xw-postgres), so stopping Docker stops the system of record too. That
took xw down on 2026-11-20 at 23:16.

### What those assumptions cost this week

- **Kernel panic, 2026-11-21 07:20** (task #595). The Mac ran at load 46
  to 70 on 30 cores, with 35.8 of 36.9 GB of swap in use. It was running
  a tier's Mac steps, host 3's linux llvm in Docker and a pushq at the
  same time. Each scheduler had admitted its own part within its own
  rules.
- **Tier 919 to 921 memory stops.** The concurrent Mac steps reached
  67215 MB and were stopped at 2026-11-20 23:18. Tier 920's reached
  65953 MB and were stopped at 02:37. Tier 921 was stopped after 1464 s
  (agents/lead-tier919-cmd.log, lead-tier920-cmd.log; `xw jobs` rows
  with rc 137). Each stop wasted 18 to 27 minutes before the steps were
  rerun one at a time.
- **Leases cut short on hosts 3 and 4** (#585, owner question #240). A
  12 minute history lease stopped a 755 s gate at Mac load 24. The
  gate's run time on a container depends on the Mac's load, which the
  lease rule does not see.
- **A tier's wall time is one host's serial queue.** Tier 920's host 2
  share ran gen, fuzz, selfhost, stdwasm, stdcov, linux cmd, linux llvm,
  sdk-stress and riscv64 one after another: 12510 s
  (lead-tier920-off-cmd.log). Hosts 3 and 4 had no part of the tier. See
  0733 for the plan it could have had.
- **Alerts over 7 days** (`xw alerts --stats`, 2026-11-21):
  - long-wait 227, waiting 125 and lease-blocked 32: jobs that sat
    behind slots while their hosts had cores and GB to spare;
  - wrong-class 54: an agent picked the wrong queue by hand;
  - hang 88: some of these are a quiet log on a job that was only slow
    because its host was oversubscribed.

## Decision (proposed)

### 1. A generic core, with Xo as the first plugin

**Core** (Workstream, project independent):

- agents, roles, tasks, decisions and asks;
- events and alerts;
- jobs and their logs and artifacts;
- machines;
- the planning scheduler (0733);
- the runner that executes jobs on machines (0735);
- the policy store (0736);
- storage (0734).

The core knows nothing about compilers, gates or tiers.

**A plugin** is a directory of data and small scripts that the core
loads:

- job types, each with its command template, needs, how it shards and
  which kinds of machine it runs on;
- batches (named groups of job types, such as a tier);
- push policy;
- alert rules;
- schedules;
- host provisioning recipes;
- result parsers that turn a log into pass, fail, and the detail worth
  keeping.

A plugin never talks to a host directly. It asks the core.

**The Xo plugin** holds Xo's hosts and job types: gate steps, tier
steps, goldens, fixed point, --ab, fuzz and stress. It also holds
pushq's rules (gate evidence, the fast checks, squash, the merge lane),
the alert rules that only make sense for Xo (sched-failed, bisect,
std-uncovered, bug-no-test) and the recipes in host-setup.

**A second plugin sketch, to test the boundary.** Take a Go web service
in its own repository, with one linux/amd64 host and GitHub pull
requests. Its job types are `go test ./...` (shard by package),
`golangci-lint` and a container build. Its batch is "pr-check", and its
push policy is "open a PR when pr-check passes", with no merge lane. It
has no tier, no seeds and no darwin.

If this plugin needs any core change beyond data, the boundary is in the
wrong place. Two examples of a misplaced boundary:

- a gate-specific field on jobs (the core's job has `needs`, `kind`,
  `shard` and `result`, nothing Xo specific);
- an Xo host name in core code.

### 2. Strong primitives, policy as data (0736)

These are rows in Workstream, not code in a script:

- machines, job types, placement and admission rules;
- alert rules, push policy, schedules and roles.

A change to any of them must be:

- validated against a schema;
- simulated against the last 7 days of jobs and events, printing what
  would have run differently and which alerts would have fired;
- recorded with who made it and why;
- reversible with one command (`xw policy rollback ID`);
- able to start in watch-only mode, where it logs its decisions without
  enforcing them.

Watch-only mode applies to one policy change at a time. It is not a
second system running beside the first (#245).

### 3. Workstream is the control plane (0735)

Agents and the lead submit intents and get results back. Examples:

- `xw run tier --rev REV`
- `xw run tests --rev REV PKG -run RE`
- `xw result JOB --detail` (the failure detail, kept by the core)

No agent runs ssh or host commands. Workstream owns:

- logins and credentials;
- host provisioning and upgrades;
- health;
- running jobs;
- progress and hang detection, from CPU time and the child processes'
  progress as well as log output;
- collecting logs and artifacts, keeping the failed step's detail;
- cleanup.

The escape hatch is a leased debug shell through Workstream: `xw shell
HOST --for 30m --why TEXT`. It is recorded, time limited and shown in
`xw status`.

### 4. One resource model, admission and placement by fit, planning for batches (0733)

- Every machine is a row with its cores, GB, disk floor, kinds and an
  optional parent. Hosts 3 and 4 have the Mac as their parent, with the
  Docker VM's share as their ceiling.
- Every job declares its needs, or inherits them from its type's
  history.
- A job starts where it fits, counting its parent.
- A batch such as the full tier is planned for the shortest finish
  time, and is replanned when a host is slow, full or down.
- When Docker stops, that is a normal event: hosts 3 and 4 are drained,
  their jobs are replanned onto other hosts, and one notice is sent
  (#244).

### 5. Storage is SQLite (0734)

- One file on the Mac's local disk, in WAL mode, with one writer: `xw
  serve` owns every write and clients go through it.
- Backups are online, and restore is one command.
- Nothing in the control plane runs in Docker.

## What each current script becomes

| Script (~/xo-lead/agents, scripts/lead) | Becomes | Notes |
|---|---|---|
| heavyd, heavy.sh, legacy-heavy.sh | core: scheduler and Mac runner | heavy.sh stays as a thin `xw run` client until its callers move, then goes; legacy-heavy.sh is deleted |
| offload.sh, offload-gate.sh, offload-gate-remote.sh, offload-amd64.sh, amd64-run.sh, offload-tier.sh | core: runner (remote) plus Xo job types | ssh, bundles, `--get` and lanes move to the runner; gate steps become job types; host routing is deleted |
| offload-prune.sh, offload-roots.sh, gocache-trim.sh, diskguard.sh, diskcheck.sh | core: cleanup and disk health | per host caps become machine rows |
| pushq.sh, pushq-env.sh | Xo plugin: push policy | the merge lane is a core lock; the fast checks are a batch |
| tier.sh, tier-mac.sh, tier-plan.sh, tier-keep.sh, tiersched.sh, riscv64-tier.sh, sdk-stress.sh | Xo plugin: the "tier" batch and its job types | the split between hosts and the memory rules are deleted (the planner does them); tier-keep becomes the core's failed-step detail |
| ctier.sh | Xo plugin: the "ctier" batch on a schedule | lease sizing is deleted |
| mainwatch.sh, mainsmall.sh, mainwasm.sh, golden-freecheck.sh, gatesteps-canary.sh, mainbisect.sh | Xo plugin: batches on schedules, plus the bisect job type | their -remote.sh halves are deleted |
| hangcheck.sh, rss-guard.sh, macload.sh, straycheck.sh, rogueproc.py | core: health and progress detection | macload's overload check becomes a guard that should never fire; rogueproc becomes the runner's knowledge of what it started |
| hourly.sh, leadduty.sh, beatrun.sh | core: schedules | |
| ab-host.sh, cgab.sh, cgcheck.sh, race-rt.sh, stdproven-offload.sh | Xo plugin: job types (exclusive timing on host 1, callgrind) | |
| host-setup/ | Xo plugin: provisioning recipes run by the core | |
| docker-up.sh | core: machine health for container hosts | |
| quiet.sh, privlock.sh | core: exclusive windows and named locks | |
| board.sh, isdone.sh, pick.sh, adr-claim.sh, code-claim.sh | core: tasks and claims | already mostly in xw |
| install.sh, launchd-install.sh | core: self install and upgrade | |
| xwhook.sh, xw-cc-hook.sh, ccguard | core: the agent-side hook | |

## Migration (in place, no parallel run: #245)

Code starts after 0.55.0 (#243). Each step cuts over as soon as its tests
pass, and the old script is removed in the same change. A step needs
only three things:

- a backup taken just before the cutover;
- a one-command way back (`xw policy rollback` or `install.sh
  --restore STEP`);
- a smoke check after the cutover.

Breakage is fixed forward.

1. **SQLite** (0734): move the data out of xw-postgres, cut over, and
   remove Postgres. This comes first because it is small and because it
   takes the control plane out of Docker.
2. **Machines and the scheduler** (0733): machines with parent
   accounting; jobs with needs; admission and placement by fit for
   heavy.sh and the lanes. heavyd's slot rules, offload-gate.sh's
   routing and tier.sh's memory rules are deleted. Owner question #240
   is answered here.
3. **Runner** (0735): offload.sh's ssh, bundle and get steps move into
   the runner, along with progress detection and the failed-step
   detail. Agents stop running ssh.
4. **The planner for batches** (0733): tier and ctier become batches
   that are planned, sharded and replanned.
5. **Policy as data** (0736): alert rules, push policy, schedules and
   roles become rows with simulation and rollback. pushq becomes the
   Xo plugin's push policy.
6. **Provisioning and credentials** (0735): host-setup and the env files
   move behind the control plane.

**Owner question #240** becomes part of step 2. On hosts 3 and 4, a
lease's time limit scales with the load on the Mac when the lease
started. It is one rule of the scheduler, not an exception to the
history lease rule.

## Consequences

- There is one place to change a rule and one model to reason about.
  Incidents name a resource ("needs 20 GB, the Mac has 11 free counting
  hosts 3 and 4") instead of a queue.
- The control plane survives the owner stopping Docker (#244). Its own
  outages and reboots are covered in 0735.
- Workstream grows a good deal: a runner, a planner and a policy store.
  The Xo specific code shrinks into a plugin directory.
- Declared needs can be wrong. Measured use and history correct them,
  and a job that grows past its declaration holds back the next start.
- With no parallel run, a bad cutover is felt at once. The backup, the
  one-command way back and the smoke check bound how long that lasts.

## Alternatives

- **Keep the schedulers and add guards** (#595's macload alert, #240's
  lease floor). This is the cheapest option, but each guard covers one
  incident, and the double counting stays.
- **Cap the Docker VM lower** (say 8 cores and 24 GB). This stops the
  double counting by hand, but leaves hosts 3 and 4 idle when the Mac is
  quiet, which the owner asked not to do (2026-11-17).
- **An outside scheduler** (Nomad, Slurm, Kubernetes). These have
  admission by fit, but tasks, decisions, the agent hooks and the alerts
  would still be Workstream's. Two systems would then have to agree
  about every machine, and Kubernetes would put the control plane back
  in containers.
- **Build the new system beside the old and move one piece at a time.**
  Ruled out by the owner (#245).
