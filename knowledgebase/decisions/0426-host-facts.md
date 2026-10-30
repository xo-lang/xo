# 0426: host facts: disk usage, resource usage, network interfaces

Status: Proposed (lead request, 2026-10-30: std-coverage gap 9, the metrics exporter's blocker; Xo lang 5). The design was agreed with the lead on 2026-10-30; the three new interface methods are BREAKING for user types implementing Fs, Proc and Net and wait for the owner's OK before they land.

## Context

ADR 0410 stage 4 gave Env.hostname, Proc.pid and Proc.ppid. An exporter also needs disk space per path (Go's syscall.Statfs, GetDiskFreeSpaceEx), CPU time and peak memory of the process and its children (syscall.Getrusage) and the host's network interfaces with their addresses (net.Interfaces, Interface.Addrs). Env.hostname answers "" on Windows.

## Decision

- `Fs.disk_usage(path) -> DiskUsage ! FsErr uses fs.read`: `DiskUsage { total, free, available, files, files_free }` (bytes and inodes; files 0 on Windows). The path resolves inside the Fs root like stat.
- `Proc.usage() -> ProcUsage` and `Proc.children_usage() -> ProcUsage`, `uses proc`: `ProcUsage { user: Duration, system: Duration, max_rss: Int }`, max_rss in bytes on every system. On Windows the runtime adds each waited child's times and peak working set, as RUSAGE_CHILDREN does on Unix.
- `Net.interfaces() -> List[NetInterface] ! NetErr uses net.listen`: index, name, mtu, hardware_addr (Str), flags (up, broadcast, loopback, point_to_point, multicast, running) and addrs as CIDR text (Str, as the rest of the prelude; std/netip parses them). An attenuated Net (Net.only) answers it: it names no remote host.
- Env.hostname on Windows: GetComputerNameExW(ComputerNamePhysicalDnsHostname), as Go's os.Hostname.

Stages, one push each: Windows hostname, disk_usage, usage, interfaces.

## Consequences

To be filled as the stages land.

## Alternatives

To be filled.

## Follow-ups

- A child's own usage on ProcOutput and Child.wait (Go's ProcessState.SysUsage, UserTime, SystemTime): not now (lead, 2026-10-30).
