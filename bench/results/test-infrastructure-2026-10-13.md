# Test infrastructure: from the Mac to EC2, queue, and warm machines (2026-10-13)

How the project's test runs moved from one Mac to EC2 over 2026-10-11 to
2026-10-13, what broke along the way, and where it landed. Figures are
from the lead's queue and agent logs (scratchpad, not committed) unless a
committed file is named.

## Stages

1. One Mac, many agents (2026-10-11): parallel agents each ran heavy
   tests; concurrent full suites exhausted 24 GB of RAM. Fix: a machine
   wide lock (one heavy run at a time) and tiers for internal/selfhost
   (short, merge, full; CLAUDE.md, toolchain.md).
2. The full tier on EC2 (2026-10-12): scripts/remote-full.sh launches a
   Graviton instance per run. Parallelizing internal/selfhost cut the
   full tier from more than 3 hours to 3,490 s on r8g.8xlarge
   (bench/results/selfhost-parallel-2026-10-12.md:60).
3. Spot to on demand (2026-10-13): spot instances were reclaimed mid run
   three times and spot capacity ran out
   (knowledgebase/gotchas/ec2-spot-reclaimed-mid-run.md); runs are on
   demand only, with the vCPU quota raised from 32 to 96 (a raise to 160
   was requested).
4. Agents' EC2 runs through a queue (2026-10-13): agents may not hold the
   SSH key; they drop a request file and the lead's runner launches it.
5. Warm machines (2026-10-13): one reusable instance per architecture,
   stopped (not terminated) after 30 idle minutes
   (scripts/warm-run.sh, knowledgebase/infrastructure/toolchain.md).
6. Platform runs at milestones (owner, 2026-10-13; CLAUDE.md): agents
   verify on the Mac; Linux and Windows run at milestones, failures are
   bisected; compiler and runtime merges pass a self rebuild fixed point
   on the Mac.

## What broke along the way (each with a gotcha)

- A sandboxed run overwrote the SSH key with nothing
  (gotchas/ec2-key-overwritten.md).
- The scripts bundled HEAD instead of a requested non-HEAD commit
  (gotchas/ec2-bundle-sent-head.md).
- A terminate call failed silently and an instance kept running
  (gotchas/ec2-terminate-not-confirmed.md).
- The EC2 full runs had no Node, so the wasm tier skipped silently
  (gotchas/ec2-full-tier-skipped-wasm.md).
- The queue runner hung on a script's watchdog holding its output pipe,
  and VcpuLimitExceeded failed jobs instead of requeueing them (fixed in
  the runner).

## Time

- Queue jobs on 2026-10-13: 19 jobs ran, about 295 minutes end to end,
  of which about 100 to 115 minutes were instance setup (5 to 6 minutes
  each: launch, toolchain, Go, clone); targeted tests themselves ran 1 to
  4 minutes.
- Warm machines: a job on a running warm instance took 15 s end to end,
  on a stopped one 32 to 67 s, against 6 to 10 minutes for a fresh
  launch (toolchain.md).
- One agent's 7.5 hours (Linux syscalls stage 2): about 4 hours blocked
  on permissions before the queue existed, about 1.5 hours waiting in the
  queue (half of it for jobs lost to runner and quota bugs), under an
  hour of EC2 run time; the rest was coding and Mac checks.
- The compiler itself: xo1 building the self hosted compiler went from
  588.86 s to 163.83 s and 26.1 GB to 9.66 GB peak
  (bench/results/xo1-speed-2026-10-13.md:23, :27), which every agent
  loop and every bootstrap check benefits from.
