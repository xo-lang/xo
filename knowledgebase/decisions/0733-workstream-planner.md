# 0733: Workstream's planning scheduler: machines, needs, admission by fit, and batch plans

Status: Proposed (xw-1, task #598, 2026-11-21). Design only. Part of
[0732](0732-one-scheduler.md), and step 2 and step 4 of its migration.
Scope: xw decisions #242 (the scheduler plans as well as admits) and
#244 (stopping Docker is a normal event).

## Context

0732 lists the seven schedulers that exist today and the incidents that
follow from each one counting only its own slice of the hardware. A full
tier is placed by hand in tier.sh:

- a fixed list of steps for the Mac;
- a fixed serial list for host 2;
- amd64 and Windows on their own hosts;
- hosts 3 and 4 left out.

Its wall time is whatever its longest fixed list happens to take.

## Decision (proposed)

### Machines

A machine is a policy row (0736). Its fields:

- `name`, `cores`, `gb`, `disk_floor_gb`;
- `kinds` (darwin, linux/arm64, linux/amd64, windows/amd64,
  windows/arm64);
- `flags`: `timing` means exclusive timing jobs only (host 1); `no:llvm`
  is host 2's rule, since its 24 GB OOM-kills clang on the llvm
  sanitizer goldens (#474);
- `parent`, with a `share` of the parent's cores and GB.

The rows as of 2026-11-21:

| Machine | Cores | GB | Kinds | Parent | Flags |
|---|---|---|---|---|---|
| mac | 30 | 96 (8 reserved for macOS and the agents) | darwin | | |
| docker-vm | 20 | 56 | (none, a parent only) | mac | |
| host3 | 20 | 56 | linux/arm64 | docker-vm | |
| host4 | 8 | 24 | linux/arm64 | docker-vm | |
| host2 | (from its probe) | 24 | linux/arm64 | | no:llvm |
| host1 | (from its probe) | (probe) | linux/arm64 | | timing |
| amd64 | 28 | (probe) | linux/amd64 | | |
| win, winarm | (probe) | (probe) | windows | | |

A child's admitted cores and GB count against its parent and every
ancestor. When hosts 3 and 4 are busy, the Mac's own jobs see that much
less room.

### Needs

A job type (Xo plugin data) declares:

- `cores`, `gb` (its peak), `minutes`;
- `kinds` and an `exclusive` flag;
- `shards` (how to split it: golden sets, packages, a fixed count);
- `after` (dependencies).

The defaults come from the type's last 30 runs in `xw jobs` and gate
history, per machine: the p90 of peak GB and minutes. A run on a host
the type has never used falls back to the type's median scaled by the
host's speed factor (below). The watchdog stops a job at 1.25 times its
gb, and the planner records the overrun for next time.

### Admission and placement by fit

A job may start on machine M when all of these hold:

- the kinds match and no flag forbids it;
- admitted GB plus the job's GB is at most M's GB minus its reserve, and
  the same holds for each ancestor;
- admitted cores plus the job's cores is at most 1.5 times M's cores, and
  the same holds for each ancestor;
- M's free disk is above its floor.

Admitted use is the larger of the declared and the measured value. The
runner's probe sends top or /proc/meminfo and the load every 10 s. So a
job that outgrows its declaration holds back the next start instead of
pushing the Mac into swap. The panic of 2026-11-21 needed 46 to 70 of
load and swap use near its limit, and this rule does not admit that.

Among the machines that fit, the scheduler places a job where its
predicted finish is earliest, using the type's per host history. The
queue keeps 0451's rules: arrival order, priority (`-p`), backfill and
the per agent cap.

### Planning a batch

A batch (the tier, ctier, pushq's fast checks) is planned as a whole.
The scheduler:

1. expands the batch into jobs, splitting a sharded type into shards
   when that shortens the critical path by more than the cost of one
   more xo build on that host;
2. sorts the jobs longest first (by predicted minutes on the fastest
   host they may use) and assigns each one to the machine where it would
   finish earliest. A job is placed only where it fits at that time,
   counting parents, and only after its dependencies end;
3. predicts the batch's wall time and prints the plan (`xw plan tier
   --rev REV`, which also runs without submitting);
4. replans the jobs that have not started whenever a running job
   overruns its predicted time by half, a machine misses two probes, a
   machine fills, or a machine leaves. A job that is running when its
   machine leaves is requeued.

Docker stopping is the case where a machine leaves (#244). The
docker-vm row goes down, which takes hosts 3 and 4 with it. Their
running jobs are requeued and placed on the Mac or host 2, and one
`machine-down` notice is sent. There is no hang, long-wait or
lease-blocked alert for them. When Docker comes back, the rows come
back up and the next replan uses them.

Time limits scale with the parent's load. A job's limit is its
predicted minutes times max(1, load at start / cores of the parent),
times 1.5. A gate on host 3 at Mac load 24 gets more time than at load
4. This answers owner question #240 in general form.

### The plan it would make for the tier, against what ran

This uses the step times that tiers 919 and 920 recorded:

- host 2's steps from lead-tier920-off-cmd.log ("moved gen: ok in 446 s,
  host memory peak 16827 of 23993 MB", and so on);
- the Mac's steps from the `xw jobs` rows of lead-tier919 and
  lead-tier920;
- amd64's steps from lead-tier920-amd64-cmd.log.

Tier 922 was still running when this was written, so tier 920 is the
latest complete one.

**What ran in tier 920.**

- **Host 2**, one step after another: build 144 s, gen 446, fuzz 131,
  selfhost 2240, stdwasm 564, stdcov 1132, linux cmd 1534, linux llvm
  3392, then sdk-stress, riscv64 and transfers (2927, the remainder of
  the total). That is 12510 s, or 3h28m, which was the tier's wall time.
- **The Mac.** The 5 big steps (arm64, llvm, cmd, corpora-all,
  corpuscheck) ran together and were stopped at 1602 s at 65953 MB. Then
  mca, dv, docs, bmem, sep, newgo and sepxo ran in 2570 s. The big
  steps had to be rerun by hand; tier 919's reruns of them, one at a
  time, took 6949 s.
- **amd64**: 589 s. **Windows**: 192 s.
- **Hosts 3 and 4**: none of the tier.

**The plan.** The Linux steps are placed longest first on host 2, host 3
and host 4. llvm is not allowed on host 2.

| Step (s on host 2) | Placed on | Ends at (s) |
|---|---|---|
| linux llvm 3392 (21.8 GB) | host3 | 3392 |
| sdk-stress, riscv64, rest 2927 | host2 | 2927 |
| selfhost 2240 (15.2 GB) | host4 (x1.6: 3584) | 3584 |
| linux cmd 1534 | host2 | 4461 |
| stdcov 1132 | host3 | 4524 |
| stdwasm 564 | host4 (902) | 4486 |
| gen 446 (16.8 GB) | host2 | 4907 |
| fuzz 131 | host3 | 4655 |

The Linux steps finish at about 4907 s, plus one xo build per host
(144 s): about 5051 s, or 84 minutes. The lower bound, with total work
divided by capacity (12366 s over 2.625 host 2 equivalents), is 4711 s.
So the plan is within 5% of the
best possible for these steps; sharding llvm (ctier already runs it as
4 leases) could close part of that gap at the cost of one xo build per
shard host, which the planner decides from the shards' own history.

Memory never exceeds the fit rule:

- hosts 3 and 4 peak together at 21.8 + 15.2 = 37 GB of the VM's 56;
- the Mac keeps 96 - 37 - 8 = 51 GB for its own steps.

The Mac's steps run two at a time by fit. Tier 919's five steps
together peaked at 67 GB, about 13 GB each, so two fit in 51 GB. Placed
longest first on two places, they finish at about 4764 s:

- **Lane A**: arm64 2542, llvm 1001 or more, corpuscheck 491, newgo
  415, sep 294, dv 12.
- **Lane B**: cmd 1825, corpora-all 1090, docs 797, mca 481, bmem 318,
  sepxo 253.

amd64 and Windows run beside these, off the critical path.

**Predicted wall time: about 85 minutes**, against 3h28m for tier 920.
That 3h28m also left its five big Mac steps undone. Tier 919 took 2h54m
on the Mac and 1h57m on host 2.

This estimate rests on assumptions that the planner replaces with
history:

- host 3 runs a step as fast as host 2, and host 4 takes 1.6 times as
  long (8 cores);
- the Mac's big steps peak at about 13 GB each when run alone;
- llvm on the Mac takes at least 1001 s (it was stopped at that point in
  tier 919);
- corpuscheck does not depend on corpora-all;
- the Mac steps are slowed by the Docker VM's 20 cores of load, and the
  replan absorbs that.

The first real check is `xw plan tier` at a tier's REV, set beside the
tier's run as the batch's first cutover (0732 migration step 4).

### What it removes

- **Rules**: heavyd's slot count, the gate free-memory percentage, the
  gate count and the load limit (fit replaces them); the lane slot
  counts and the memory lease takes all slots rule; offload-gate.sh's
  host 4 first, reroute and history lease rules; tier.sh's split of
  steps between hosts, its concurrent-then-serial Mac fallback and
  TIER_MAX_GB; ctier.sh's lease sizing.
- **Alerts**: wrong-class goes, since there is no class to pick.
  long-wait, waiting and lease-blocked become one `waits-for-fit` alert
  that names the resource, raised only when a job could never fit or
  has waited past its plan by more than half. hang uses the job's scaled
  limit and the runner's progress probe (0735).

## Consequences

- A tier's wall time becomes a plan output that can be checked, not the
  length of one host's fixed list.
- The plan is only as good as its history. A new job type runs with
  conservative defaults (the largest of its class) until it has 3 runs.
- A replan that moves a job to a slower host can make the batch longer
  than its first prediction. `xw plan --explain` shows each move and
  why it was made.

## Alternatives

- **Admission only, with no planning.** This is simpler and already
  stops the memory incidents, but the tier keeps its 3h28m serial host 2
  list. Ruled out by #242.
- **An exact scheduler** (an ILP or constraint solver over the batch).
  It gives a better plan in theory, but the step times vary by more than
  the gap between a longest-first plan and the best one; above, that
  gap is 4% from the lower bound.
- **Static plans per batch, edited by hand.** These go out of date with
  every host change, which is what tier.sh's split is today.
