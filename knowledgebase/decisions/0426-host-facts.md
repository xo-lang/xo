# 0426: host facts: disk usage, resource usage, network interfaces

Status: Proposed (lead request, 2026-10-30: std-coverage gap 9, the metrics exporter's blocker; Xo lang 5). The design was agreed with the lead on 2026-10-30; the three new interface methods are BREAKING for user types implementing Fs, Proc and Net and wait for the owner's OK before they land.

## Context

ADR 0410 stage 4 gave Env.hostname, Proc.pid and Proc.ppid. An exporter also needs disk space per path (Go's syscall.Statfs, GetDiskFreeSpaceEx), CPU time and peak memory of the process and its children (syscall.Getrusage) and the host's network interfaces with their addresses (net.Interfaces, Interface.Addrs). Env.hostname answers "" on Windows.

## Decision

- `Fs.disk_usage(path) -> DiskUsage ! FsErr uses fs.read`: `DiskUsage { total, free, available, files, files_free }` (bytes and inodes; files 0 on Windows). The path resolves inside the Fs root like stat.
- `Proc.usage() -> ProcUsage` and `Proc.children_usage() -> ProcUsage`, `uses proc`: `ProcUsage { user: Duration, system: Duration, max_rss: Int }`, max_rss in bytes on every system. On Windows the runtime adds each waited child's times and peak working set, as RUSAGE_CHILDREN does on Unix.
- `Net.interfaces() -> List[NetInterface] ! NetErr uses net.listen`: index, name, mtu, hardware_addr (Str), flags (up, broadcast, loopback, point_to_point, multicast, running) and addrs as CIDR text (Str, as the rest of the prelude; std/netip parses them). An attenuated Net (Net.only) answers it: it names no remote host.
- Env.hostname on Windows: GetComputerNameExW(ComputerNamePhysicalDnsHostname), as Go's os.Hostname.
- `Proc.cpus() -> Int` and `Proc.memory() -> Int`, `uses proc` (xo-lang-4's stage, lead 2026-10-30): the processors this process may run on, as Go's runtime.NumCPU (Linux sched_getaffinity, else /sys/devices/system/cpu/online; macOS hw.activecpu; Windows the active processors of every group: the runtime's own count, llrt poolsetup.xo cpu_count, read once at the pool's setup, so a call is a load), and the machine's physical memory in bytes (hw.memsize, /proc/meminfo MemTotal, GlobalMemoryStatusEx; GlobalMemoryStatusEx joins compiler/macho/peimports.xo before a seed's build can use it). Why: compiler/xo sizes its jobs with a sysctl, lscpu and nproc probe per call (emit_jobs), 18 processes in an HTTP edit build on the arm64 offload host; the same build with the job count preset is 0.874 of the time (xo bench --ab, 100 rounds, 88.96 to 77.71 ms). The physical core count decision 0220 sizes by stays in compiler/xo (sysfs topology, hw.physicalcpu), not std. Linux CPU quotas (a cgroup's cpu.max) are not counted, as Go's runtime.NumCPU does not count them.

Stages, one push each: Windows hostname, disk_usage, usage, interfaces. Then cpus and memory (xo-lang-4): the methods, compiler/xo's job count read from them once per process.

## Consequences

- Breaking for user types implementing `Fs`, `Proc` or `Net` (as `Net.dial_timeout` and `Env.hostname` were): each adds the new methods. The pushes wait for the owner's OK of the CHANGELOG's BREAKING lines.
- Four prelude structs (`DiskUsage`, `ProcUsage`, `NetInterface`, `InterfaceFlags`), each declared again in std/native/prelude.xo: the native SSA builder treats a prelude struct as opaque unless the native prelude declares its implementation, and the seed knows no other way (gotchas/new-prelude-struct-needs-native-twin.md).
- Windows keeps no children's usage, so the native runtime adds each waited child's times and peak working set in rt.static_word 95 to 97 (windows.reap); the Go backend adds the times only (os/exec has closed the handle by then), so its children's `max_rss` is 0 on Windows.
- Linux's interface list comes from netlink with no libc, as Go's; macOS from getifaddrs; Windows from IPHLPAPI's GetAdaptersAddresses through std/native/llrt/winadapters.xo, which decision 0427's DNS configuration added first.
- The Windows imports the stages call from code xo's own build reaches (GetDiskFreeSpaceExW, GetProcessTimes) landed in compiler/macho/peimports.xo first (cd4162774, before the 0.3x seed of 2026-10-30): a builder whose import table lacks them cannot link the next xo on windows/amd64 (gotchas/seed-older-than-intrinsics.md). GetProcessTimes takes five arguments, more than rt.call_c4 passes, so a run-time lookup (crash.xo's k32_proc) was not an option.
- A test's MemFs answers `set_disk_usage`'s value; FakeNet one loopback interface; FakeProc the real process's usage (FakeProc is the Proc natively).

## Alternatives

- Module functions in a std module (`host.disk_usage(fs, path)`) instead of capability methods: not breaking, but the capability would be an argument the function only forwards, and attenuation (`Fs.sub`, `Net.only`) is per capability. Rejected for consistency with `Env.hostname` and `Proc.pid`.
- Addresses as a prelude `IpPrefix` type: the prelude does not use std/netip and no other prelude API does; `netip.Prefix.parse` reads the text (lead, 2026-10-30).
- `Net.only` denying `interfaces()` unless its list names it: the list names no remote host, so there is nothing for the attenuation to refuse (lead, 2026-10-30).
- `FakeNet.set_interfaces`: not built; the fixed loopback covers the tests seen so far.

## Follow-ups

- A child's own usage on ProcOutput and Child.wait (Go's ProcessState.SysUsage, UserTime, SystemTime): not now (lead, 2026-10-30).
