# 0426: host facts: disk usage, resource usage, network interfaces

Status: Accepted (owner, 2026-10-30, through the lead: the BREAKING stages land in one breaking batch in 0.36, with Xo Lang 4's Proc.cpus and Proc.memory as a further 0426 stage; code waits for the seed cut). Was: Proposed (lead request, 2026-10-30: std-coverage gap 9, the metrics exporter's blocker; Xo lang 5). The design was agreed with the lead on 2026-10-30; the three new interface methods are BREAKING for user types implementing Fs, Proc and Net and wait for the owner's OK before they land.

## Context

ADR 0410 stage 4 gave Env.hostname, Proc.pid and Proc.ppid. An exporter also needs disk space per path (Go's syscall.Statfs, GetDiskFreeSpaceEx), CPU time and peak memory of the process and its children (syscall.Getrusage) and the host's network interfaces with their addresses (net.Interfaces, Interface.Addrs). Env.hostname answers "" on Windows.

## Decision

- `Fs.disk_usage(path) -> DiskUsage ! FsErr uses fs.read`: `DiskUsage { total, free, available, files, files_free }` (bytes and inodes; files 0 on Windows). The path resolves inside the Fs root like stat.
- `Proc.usage() -> ProcUsage` and `Proc.children_usage() -> ProcUsage`, `uses proc`: `ProcUsage { user: Duration, system: Duration, max_rss: Int }`, max_rss in bytes on every system. On Windows the runtime adds each waited child's times and peak working set, as RUSAGE_CHILDREN does on Unix.
- `Net.interfaces() -> List[NetInterface] ! NetErr uses net.listen`: index, name, mtu, hardware_addr (Str), flags (up, broadcast, loopback, point_to_point, multicast, running) and addrs as CIDR text (Str, as the rest of the prelude; std/netip parses them). An attenuated Net (Net.only) answers it: it names no remote host.
- Env.hostname on Windows: GetComputerNameExW(ComputerNamePhysicalDnsHostname), as Go's os.Hostname.
- `Proc.cpus() -> Int` and `Proc.memory() -> Int`, `uses proc` (xo-lang-4's stage, lead 2026-10-30): the processors this process may run on, as Go's runtime.NumCPU (Linux sched_getaffinity, else /sys/devices/system/cpu/online; macOS hw.activecpu; Windows the active processors of every group: the runtime's own count, llrt poolsetup.xo cpu_count, read once at the pool's setup, so a call is a load), and the machine's physical memory in bytes: llrt heapos.phys_mem, which debug.mem_total already returns (hw.memsize, sysinfo, GlobalMemoryStatusEx; the Windows branch is xo-lang-4's l4-physmem, after the 0.35 seed, its import row landed ahead of it in d6b04f6e6). Why: std code and compiler/xo read these facts without a process; compiler/xo's job count reads them once per process. The 18 probe processes an HTTP edit build ran (emit_jobs: sysctl, lscpu, nproc per call) are gone before this stage: xo-lang-4's l4-nospawn computes the cores once (Linux sysfs, Windows NUMBER_OF_PROCESSORS, one sysctl on macOS); A its parent: linux/arm64 0.879 and darwin/arm64 0.959 (xo bench --ab), windows/amd64 0.871 (hand timed, 30 interleaved rounds: xo bench needs /usr/bin/time there; all the job count, which the old probe had at 1; with the jobs pinned equal 1.002). This stage removes compiler/xo's last macOS sysctl. The physical core count decision 0220 sizes by stays in compiler/xo (sysfs topology, hw.physicalcpu), not std. Linux CPU quotas (a cgroup's cpu.max) are not counted, as Go's runtime.NumCPU does not count them.

Stages, one push each: Windows hostname, disk_usage, usage, interfaces. Then cpus and memory (xo-lang-4, after l4-physmem and l4-nospawn): the methods, then compiler/xo's job count read from them once a seed carries them (gotchas/seed-older-than-intrinsics.md).

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

## Shipped

Xo lang 5's stages, one push each, as decided (2026-10-30):

- Env.hostname on Windows: ef403c1dd (golden env_hostname "named true" on every host but WASI).
- Fs.disk_usage: cbfad94d9.
- Proc.usage and Proc.children_usage: 0b2929599 (golden host_usage; --ab against Go's syscall.Getrusage, wall 1.188, std/changes/prelude.md).
- Net.interfaces: 72d46e4df (golden host_ifaces on arm64, LLVM, Go parity, the linux offload hosts and native Windows; --ab against Go's net.Interfaces plus Interface.Addrs, wall 0.254: Xo reads every address in one netlink dump, Go one per interface).

Each updated the user_caps golden and the CHANGELOG's Breaking lines; pushq's prelude check also caught cmd/xo/testdata/nogc/userfs's BlockNet, a user Net, which forwards interfaces to its inner Net. Proc.cpus and Proc.memory remain xo-lang-4's stage.

## Follow-ups

- A child's own usage on ProcOutput and Child.wait (Go's ProcessState.SysUsage, UserTime, SystemTime): accepted by the owner on 2026-10-30, see the addendum below.

## Addendum (2026-10-30): a child's own usage

Status: Accepted (owner, 2026-10-30), proposed by xo-lang-4 the same
day; it matches Go's ProcessState.UserTime, SystemTime and SysUsage.
Proof required: an `xo bench --ab` of a small and a large program on
darwin, linux/arm64 and native Windows, each side showing its own peak.

### Context

`Proc.children_usage()` sums the waited children's CPU time, but its
`max_rss` is the largest peak of every child waited so far
(RUSAGE_CHILDREN's ru_maxrss; on Windows the reap keeps the largest peak
working set, rt.static_word 97). A caller that runs several children
cannot tell one child's peak from another's. `xo bench --ab` is the
first such caller: on Windows it now reads cpu_ms as children_usage's
growth across a round (exact, since it runs one child at a time), but a
round's max_rss_kb would be the larger side's peak from the first round
on, so it is not measured there. On darwin and Linux bench still parses
/usr/bin/time's output for both.

### Decision

- `ProcOutput` gains `usage: ProcUsage`: the child's own user and system
  CPU time and peak resident set (bytes), as the run that waited for it
  read them.
- `Child.wait_usage() -> (Int, ProcUsage) ! ProcErr` (`uses proc`):
  wait's exit status with the same figures, for a spawned child.
  (`Child.wait` keeps its signature.)
- Sources: Linux and darwin `wait4` (the reap already waits; it passes
  an rusage buffer: ru_utime, ru_stime, ru_maxrss in KiB on Linux and
  bytes on darwin, scaled to bytes); Windows GetProcessTimes and
  K32GetProcessMemoryInfo's PeakWorkingSetSize on the child's handle
  before it is closed (the reap reads both today to add them to the
  children's totals); WASI zeros, as Proc.usage. Go backend:
  ProcessState.UserTime, SystemTime, and SysUsage's Maxrss
  (syscall.Rusage; Windows: the same calls on the process handle).
- A fake Proc (the test world, an attenuated Proc) returns zeros.
- `xo bench` reads `usage` on every OS: cpu_ms and max_rss_kb from it,
  and /usr/bin/time stays only on darwin, for `time -l`'s instructions
  and cycles. Linux and Windows run the child alone. The Windows note
  under the table goes.

### Consequences

- Not breaking: the field has a default (`usage: ProcUsage =
  ProcUsage{user: 0s, system: 0s, max_rss: 0}`, as Request's fields
  have), so a `ProcOutput` literal without it (a fake) still compiles,
  and Proc's method signatures do not change. The Go backend's
  generated ProcOutput gains the field (std/rt std_gen_prelude.go).
- Bench's figures change source on Linux (GNU time's figures come from
  the same wait4, so they should match; checked before the switch on
  darwin and linux by running both on one program, 20 rounds each).
- Spec std 3 (Proc), Appendix A (stdsync), std/changes/prelude.md with
  an --ab timing row against Go's os/exec + ProcessState.

### Alternatives

- Keep children_usage only and difference it per child: exact for CPU,
  impossible for the peak (a max is not differenced).
- Bench opens the child itself through a Windows-only runtime call: a
  private path std users would not have.
- Keep GNU time on Linux and darwin: two parsers stay, and Windows still
  lacks memory.
