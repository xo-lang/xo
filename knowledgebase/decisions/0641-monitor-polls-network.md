# 0641: the monitor does the poller thread's fallback poll (rejected)

Status: Rejected (measured, 2026-11-14; Xo Lang 2, the lead agreed)

## Context

Decision 0640 left one or two short throttles in some 12 s runs of
bench/perf/http under a 2 CPU quota (CPUQuota=200%, 128 connections, the
linux/amd64 offload host). With the threads named per role in an
experiment build, the windows a throttle landed in showed the root and the
one worker at 96% each and two helper threads at about 0.2 ms each: the
scheduler's monitor (pool.xo mon_main) and the network poller thread
(poll.xo poll_main), both ticking every 10 ms. Go has one such thread:
sysmon polls the network itself when nothing has for 10 ms, and idle Ms
block in the poller. Xo's idle workers already block in the poller
(decision 0324), so the poller thread's remaining roles were its fallback
poll while every worker is busy and the waits of a quiet program.

## Decision (proposed, then rejected)

The monitor polls without waiting on each tick when no one has polled for
its current sleep and the poll token is free (poll.xo np_mon, through
cabi.xo; its readied fibers go through the inject queue with wakes), and
looks again 0.5 ms later when the poll found events (the poller thread's
NP_TICK_MIN). The poller thread sleeps while the monitor is awake
(pool.xo mon_poll_wait on the monitor record's M_PWAIT word) and the
monitor wakes it when it parks, so the poller's own tick covers the
monitor's sleep, and nothing changes without a monitor (XO_DEFER_WAKE=0).

## Results

linux/amd64 host, CPUQuota=200%, c=128, 8 pairs, the two builds alternated
first and second:

| | before | the monitor polls |
|---|---|---|
| poller thread CPU (ticks of 10 ms a run) | 2 | 0 |
| throttles (runs with any) | 2 (2 of 8) | 3 (2 of 8) |
| rps | 127.7k to 137.7k | 125.7k to 141.9k |

The throttles did not fall. With no monitor thread at all
(XO_DEFER_WAKE=0), 8 runs of each build throttled 1 and 3 times. About one
throttle of 0.1 to 0.5 ms in a quarter of the runs is left whichever helper
threads run, once two threads use 96% of the quota: the kernel hands quota
to each CPU in 5 ms slices (sched_cfs_bandwidth_slice_us 5000), and any
brief run on a third CPU strands part of one. At a fixed rate equal to
Go's throughput (70k and 88k rps), Xo shows 0 throttles in 12 runs at 115%
to 150% CPU (decision 0640's residual).

## Consequences

The code is not merged (the experiment's diff is kept in the agent's notes,
mon/0641-experiment.diff). The poller thread and the monitor stay separate;
0640's residual names the CFS slice floor at saturation instead.

## Alternatives

- A cap below the quota (for example 95% of it) to leave headroom: 0
  throttles at saturation, but fewer requests served for a 0.1 to 0.5 ms
  stall; rejected by the lead.
- Waking a worker only while the awake workers are under the cap, not the
  alive ones: the wakes and throttles were unchanged.
