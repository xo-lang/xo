# std/dns: a one-query lookup on the netpoller while other tasks exist (2026-11-03)

Host 1 (linux/arm64), alone (exclusive lease), `xo bench --ab --rounds 15`, A = main at 720fb3a67 (the
lookup waits in ppoll on the thread), B = the change. Workloads, 2,000 MX lookups each:

- mx: bench/perf/dns mx, the server a task of the same program;
- ext: bench/perf/dns `ext 127.0.0.1:18053`, a Go responder in another process; tasks in the code, one at run time;
- lone: a lookup-only program (no task, scope or channel in its code) against the same responder.

## Netpoller for every one-query lookup (31ba6938b re-applied)

| workload | A wall (ms) | B wall (ms) | B / A (95% interval) | A CPU (ms) | B CPU (ms) |
|---|---|---|---|---|---|
| mx | 103.7 | 18.1 | 0.175 (0.173 to 0.177) | 86.36 | 18.03 |
| ext | 89.95 | 84.48 | 0.939 (0.935 to 0.972) | 36.83 | 74.13 |
| lone | 88.17 | 84.07 | 0.954 (0.944 to 0.98) | 36.13 | 73.46 |

A lone task's lookup took twice the CPU on the netpoller for 5% less wall time.

## Netpoller only while pool.others_active (the change)

| workload | A wall (ms) | B wall (ms) | B / A (95% interval) | A CPU (ms) | B CPU (ms) |
|---|---|---|---|---|---|
| mx | 103.7 | 18.13 | 0.175 (0.173 to 0.177) | 86.63 | 18.15 |
| ext | 89.68 | 89.38 | 0.997 (0.982 to 1.006) | 36.81 | 36.49 |
| lone | 89.73 | 89.63 | 0.999 (0.992 to 1.007) | 36.48 | 36.48 |

Peak memory: mx 0.926, ext and lone 1.0.

## Twin row against Go (bench/perf/dns, twin.go.txt)

Host 1, alone, `xo bench --ab --rounds 15`, A = Go 1.26.1's twin (net.Resolver, PreferGo), B = Xo with the change
(load average 4.4 before the run, no round loaded):

| workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|
| none | 0.949 | 0.657 | 0.692 | 0.649 to 0.779 | 0.162 |
| mx | 20.87 | 19.38 | 0.928 | 0.901 to 0.955 | 0.1 |

mx was 4.657 (bench/results/std-twin-rows2-2026-11-03.md). The twin had no warm-up problem: `xo bench --ab`
drops its first round, and main's 103.7 ms there was the steady state of the thread's ppoll wait.
