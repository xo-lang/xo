# 0732: one scheduler that owns every machine

Status: Proposed (xw-1, task #598, 2026-11-21; the owner asked for one
scheduler that owns every machine). Design only: no code until the owner
decides. Builds on 0442 (heavyd) and 0451 (heavyd lanes); would supersede
the admission parts of both once its last stage lands.

## Context

Seven schedulers decide when work runs, and each one models a different
slice of the hardware. None of them knows what the others have admitted,
and two of them count the same RAM.

### Inventory (as read on 2026-11-21)

| Scheduler | What it admits | Resource model it assumes |
|---|---|---|
| heavyd Mac classes (0442; scripts/lead/heavyd/config.go) | gate, bulk, measure, quick runs on the Mac | the Mac alone: 3 heavy slots, a gate starts at 35% free memory, fewer than 5 gates, load under 48, 20 s stagger; a job's tree stopped past HEAVY_MAX_GB (32 by default) |
| heavyd lanes (0451; ~/xo-lead/heavyd-lanes.conf) | offload.sh, offload-gate.sh, amd64-run.sh, pushq's merge, docker runs | slots per host, in arrival order: arm64 (host 1) 3, pass2 3, pass3 3, pass4 2, amd64 6, win 2, winarm 1, merge 1, docker 1; a slot has no size, a "memory" lease takes every slot |
| offload-gate.sh routing | which host a gate takes | memory gates go to host 4 first, short gates to host 4 when hosts 2 and 3 are held whole, lease minutes from gate history (#585) |
| tier.sh and tier-mac.sh | the 8 hour tier's Mac steps | its own floors: 50 GB free disk to start, TIER_MAX_GB 40 for a step (the 64 GB HEAVY_MAX_GB seen in tiers 919 and 920), one Mac step at a time |
| ctier.sh | continuous tier steps | one lease per step, minutes from a floor or 1.3 times the slowest run, llvm split into 4 leases |
| pushq.sh | pushes | one merge lease; its fast checks run as Mac gate jobs |
| xw schedule (xw-schedule.conf) | 11 recurring jobs (tier, ctier, mainsmall, mainwatch, ...) | time only: it starts a job when due, whatever the machines are doing |

Hosts 3 and 4 are Docker containers on the Mac. The Docker VM is set to
20 of the Mac's 30 cores and 57328 MiB of its 96 GB (Docker Desktop
settings-store.json). heavyd's Mac rules see that VM only as "used"
memory and load, and the lanes see hosts 3 and 4 as separate machines
with their own slots. So the Mac can be admitted three times over: Mac
gates by free %, a tier Mac step by its own cap, and up to 5 slots of
container work.

### What those assumptions cost this week

- **Kernel panic, 2026-11-21 07:20** (task #595): load 46 to 70 on 30
  cores, swap 35.8 of 36.9 GB, from a tier's Mac steps, host 3's linux
  llvm in Docker and a pushq at once. Every scheduler had admitted its own
  part within its own rules.
- **Tier 919 to 921 memory stops**: tier 919's Mac step reached 67215 MB
  and was stopped at 2026-11-20 23:18 (exit 137), tier 920's 65953 MB at
  02:37 (agents/lead-tier919-cmd.log, lead-tier920-cmd.log). A step capped
  at 64 GB on a 96 GB Mac whose Docker VM may hold 56 GB has no room.
- **Leases cut short on hosts 3 and 4** (#585, owner question #240): a 12
  minute history lease stopped a 755 s gate at Mac load 24, because the
  gate's time on a container depends on the Mac's load, which the lease
  rule does not see.
- **7 day alert counts** (`xw alerts --stats`, 2026-11-21): long-wait
  227, waiting 125, hang 88, wrong-class 54, lease-blocked 32. long-wait,
  waiting and lease-blocked are jobs queued behind slots that had room
  in cores or GB; wrong-class is an agent picking the wrong queue by
  hand (a 10 minute run in the Mac gate class); part of hang is a quiet
  log on a job that was only slow because its host was oversubscribed.

## Decision (proposed)

One scheduler, heavyd, owns one resource model of every machine, and
every producer submits to it. Admission is by fit, not by slot count.

1. **Machines** (`~/xo-lead/machines.conf`, read every pass): one line
   per machine with cores, GB of memory, disk floor, kinds (darwin,
   linux/arm64, amd64, windows) and a `parent` for a machine that lives
   inside another. Host 3 and host 4 name the Mac as their parent and
   the Docker VM's share (20 cores, 56 GB) as their ceiling, so every GB
   they take is also taken from the Mac. Host 1 keeps a `timing` flag:
   only exclusive timing jobs.
2. **Jobs declare needs**: `cores`, `gb` (the peak it may reach; heavyd's
   watchdog stops it past that, as HEAVY_MAX_GB does today), `minutes`
   (expected, and the limit), `kind` (darwin, linux/arm64, any linux,
   amd64), and `exclusive` for timing. heavy.sh, offload.sh,
   offload-gate.sh, tier.sh, ctier.sh, pushq.sh and the xw schedule pass
   them as `HEAVY_NEED=cores=8,gb=20,min=30,kind=linux/arm64`. A job
   that declares nothing gets its class's default, taken from the last
   30 runs of the same name in `xw jobs` (peak GB, p90 minutes), so no
   caller has to guess.
3. **Admission by fit**: a job starts on the first machine of its kind
   where the sum of admitted `gb` plus its own stays under the machine's
   GB minus a reserve (8 GB on the Mac for macOS and the agents), and
   admitted cores stay under 1.5 times its cores; for a child machine
   the same sum must also fit its parent. Arrival order with the 0451
   priority and backfill rules stays. Measured use (top, the host's
   /proc/meminfo through offload's probe) replaces the declaration when
   it is higher, so a job that grows past its declaration holds back the
   next start instead of driving the Mac into swap.
4. **Routing is heavyd's**: a job asks for a kind, not a host or a
   class. heavyd picks the host, which removes the hand choice that
   wrong-class alerts flag and the reroute rules in offload-gate.sh.
   A job that needs one host for a put and a run (ab-host.sh, tier.sh)
   keeps naming it, as `env:FILE` does now.
5. **Time limits from the machine's state**: a job's limit is its
   expected minutes scaled by the load on its machine (and its parent)
   when it started, so a gate on host 3 at Mac load 24 gets the time it
   needs. That is the general form of owner question #240.
6. **One view**: `heavy.sh --status` and `xw jobs` show each machine's
   admitted and measured cores and GB, and why each waiter waits ("needs
   20 GB, Mac has 11 free counting hosts 3 and 4").

### What it removes

- The panic and the tier memory stops: the Mac cannot be admitted past
  its GB once hosts 3 and 4 count against it.
- The Mac gate rules (35% free, 5 gates, load 48, stagger) and tier.sh's
  own memory floor become one fit rule.
- The memory-lease takes-every-slot rule, offload-gate.sh's host 4 first
  and reroute rules, and the per-lane CAP become fit plus the per-agent
  cap.
- Alerts: wrong-class goes (no class to pick); long-wait, waiting and
  lease-blocked become one "waits for fit" alert naming the resource;
  hang on a quiet log is judged against the job's scaled limit;
  macload.sh's mac-overload (#595) stays as a guard that should never
  fire.

## Migration

Each stage is pushed and live before the next, as in 0451.

1. **Shadow model**: machines.conf, needs taken from `xw jobs` history,
   and heavyd logging what fit admission would have done beside what it
   did. A week of shadow logs, then the owner reviews.
2. **Parent accounting**: hosts 3 and 4 count against the Mac in the
   Mac gate rule (the smallest change that stops the panic).
3. **Lanes by fit**: lane leases are admitted by fit; slot counts stay as
   an upper bound, then are removed.
4. **Producers declare needs**: tier.sh, ctier.sh, pushq.sh and the xw
   schedule pass HEAVY_NEED; offload-gate.sh drops its routing.
5. **Delete** the old rules and alerts listed above.

**Owner question #240** becomes stage 2 plus decision 5: leases on hosts
3 and 4 scale with the Mac's load, as one rule of the scheduler rather
than an exception to the history lease rule.

## Consequences

- One place to change a rule and one model to reason about; incidents
  name a resource instead of a queue.
- Declarations can be wrong; measured use and history keep the model
  honest, but the first week needs the shadow log to set the reserve.
- heavyd becomes a harder dependency: when it is down every producer
  falls back to its old lock, as 0451's exit 75 does today, and the
  fallbacks stay until stage 5.
- More Go in heavyd (fit, parent sums, history reads); the queue rules
  of 0451 stay.

## Alternatives

- **Keep the schedulers and add guards** (#595's macload alert, #240's
  lease floor): cheapest, but each guard covers one incident, and the
  double counting stays.
- **Cap the Docker VM lower** (say 8 cores, 24 GB): stops the double
  count by hand, but leaves hosts 3 and 4 idle when the Mac is quiet,
  which the owner asked not to do (2026-11-17).
- **Move hosts 3 and 4 off the Mac**: removes the parent relation, but
  needs hardware, and the model is still split across schedulers.
- **An outside scheduler** (Nomad, Slurm): fit admission exists there,
  but the lock-file mirror, quiet windows, measure drains and the xw
  events that hangcheck, pushq and the alerts read would all be rewritten.
