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

The rows as of 2026-11-21 (cores and GB checked by #599):

| Machine | Cores | GB | Kinds | Parent | Flags |
|---|---|---|---|---|---|
| mac | 30 | 96 (8 reserved for macOS and the agents) | darwin | | |
| docker-vm | 20 | 56 | (none, a parent only) | mac | |
| host3 | 12 (cgroup; `nproc` says 20) | 31 | linux/arm64 | docker-vm | |
| host4 | 8 | 24 | linux/arm64 | docker-vm | |
| host2 | 18 | 22 | linux/arm64 | | no:llvm |
| host1 | 12 | 39 | linux/arm64 | | timing |
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

**The inputs, checked against history (task #599, 2026-11-21).**

- **Cores and memory.** Each was read from a light lease on the host on
  2026-11-21: `nproc`, `free -m`, and the cgroup's `cpu.max` and
  `memory.max`.
  - host 2: 18 cores, and a 22 GiB cgroup cap on a 23.4 GiB host.
  - host 3: `nproc` reports 20, but `cpu.max` caps it at 12 cores and
    `memory.max` at 31 GiB. `free` shows the whole VM's 56 GB.
  - host 1: 12 cores and 39 GiB, with no cgroup cap.
  - host 4: offload.md gives `--cpus 8 --memory 24g`. Its probe found no
    free slot in 2 minutes, because two ctier llvm leases held its lane.
- **Speed against host 2.** This is the p50 per gate step over the 1648
  gate logs of 2026-11-14 to 2026-11-21, with the host taken from the
  log's CPU count and its lease lane. Gates run up to 3 steps at once,
  so these are confounded by whatever ran beside them.
  - host 3: llvm 0.80, stdcov 0.88, wasm 1.19, cmd 1.27 and arm64 1.34.
    That is about 1.2 overall, not 1.0.
  - host 4: wasm 1.25, llvm 1.65 and arm64 2.93. That is about 2 overall,
    not 1.6.
  - host 1 is faster than host 2 on most steps (llvm 0.75, cmd 0.92,
    stdcov 0.77), but it is flagged `timing`.
- **The Mac's big steps peak far above 13 GB each.** heavy.sh's peaks
  for tier 919's reruns, one at a time, were:
  - arm64 25985 MB;
  - cmd 27240 MB;
  - corpora-all 4422 MB;
  - corpuscheck 3205 MB;
  - llvm 33828 MB. That is over HEAVY_MAX_GB=32, which is why it was
    stopped at 1001 s, not a timeout.

  The 67 GB of five steps together came from the steps not peaking at
  the same moment.
- **Host 2's peaks** (tier 920's off-cmd log): gen 16827 MB, llvm 21752,
  selfhost 15228, cmd 13529 and stdwasm 12283, all of 23993 MB.

**The plan, redone with these inputs.** The Linux steps are placed
longest first, with host 3 at 1.2 times host 2 (llvm 0.8 and stdcov 0.88
where measured) and host 4 at 2 times. llvm is not allowed on host 2.

| Step (s on host 2) | Placed on | Ends at (s) |
|---|---|---|
| linux llvm 3392 (21.8 GB) | host3 (x0.8: 2714) | 2714 |
| sdk-stress, riscv64, rest 2927 | host2 | 2927 |
| selfhost 2240 (15.2 GB) | host4 (x2: 4480) | 4480 |
| linux cmd 1534 | host2 | 4461 |
| stdcov 1132 | host3 (x0.88: 996) | 3710 |
| stdwasm 564 | host3 (677) | 4387 |
| gen 446 (16.8 GB) | host2 | 4907 |
| fuzz 131 | host3 (157) | 4544 |

The Linux steps still end at about 4907 s, plus one xo build per host:
about 5051 s, or 84 minutes. Host 2's own list sets that end, so the
slower hosts 3 and 4 do not change it.

Memory stays within the caps:

- host 3's llvm uses 21.8 GB of its 31 GiB;
- host 4's selfhost uses 15.2 GB of its 24;
- together they use 37 GB of the VM's 56;
- the Mac keeps 96 - 37 - 8 = 51 GB for its own steps.

**The Mac is now the critical path.** arm64 (26 GB) and cmd (27 GB) do
not fit together in 51 GB, and darwin llvm needs about 34 GB alone (42
GB with the 1.25 watchdog margin). So the three big steps run one after
another, beside the small ones:

- **Lane A**: arm64 2542, cmd 1825, then llvm, for 4367 s plus llvm.
- **Lane B**: corpora-all 1090, docs 797, corpuscheck 491, mca 481,
  newgo 415, bmem 318, sep 294, sepxo 253 and dv 12, for 4151 s.

During llvm, lane B's steps (at most 4.4 GB) still fit. Once hosts 3
and 4 end near 4500 s, the Mac gains their 37 GB. No complete darwin
llvm run is recorded in the last 7 days of `xw jobs`; it ran for more
than 1001 s before it was stopped.

**Predicted wall time: at least 90 minutes** (4367 + 1001 = 5368 s),
plus whatever darwin llvm takes past 1001 s. That is against 3h28m for
tier 920, which also left its five big Mac steps undone. Sharding darwin
llvm as ctier shards linux llvm (4 leases) is what would bring the Mac
lane below the Linux one, and the planner decides it from the shards'
own peaks.

These assumptions remain unverified:

- darwin llvm's full time and its per-shard peak;
- corpuscheck does not depend on corpora-all;
- the gate step factors carry over to the tier's steps.

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
  the gap between a longest-first plan and the best one; in 0733's
  first plan, that gap was 4% from the lower bound.
- **Static plans per batch, edited by hand.** These go out of date with
  every host change, which is what tier.sh's split is today.
