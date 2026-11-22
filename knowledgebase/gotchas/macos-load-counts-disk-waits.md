# macOS load average counts threads waiting on the disk

Status: Observed 2026-11-22 (task #690)

Symptom: heavyd held every new Mac gate and slot run for 20 minutes and
more ("admission v3: load 139.89 over 1.5 x 30 cores (45)"; alerts
128510, 128603, 128604) while top showed the CPU 82% idle. From 06:15 to
06:40 the 1-minute load stood near 117 with mds_stores indexing at about
one core during tier927's Mac step.

Cause: macOS's load average counts threads blocked in uninterruptible
waits (disk I/O, indexing) as running, so it climbs far past the core
count while the CPUs sit free. Every heavyd rule that held on load (the
admission policy's refuse_load_per_core, HEAVY_GATE_MAX_LOAD, #661's
HEAVY_LOAD5_HOLD) read a busy Mac where the CPU was free.

Fix: heavyd samples the CPU busy percent (`top -l 2 -n 0 -s 2` every
10 s on macOS, /proc/stat on Linux; scripts/lead/heavyd probe.go
CPUBusy). While a sample is known (under a minute old), new runs wait on
the CPU being at least 90% busy (HEAVY_CPU_BUSY_MAX, policy
cpu_busy_max_pct), and the load counts only past a secondary cap of 6 x
the cores (HEAVY_LOAD_CAP_PER_CORE, policy load_cap_per_core). With no
sample, the load rules stand. TestCPUBusyAdmitsOverLoad covers it.
