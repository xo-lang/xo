# Linux system calls from Xo, and Linux programs with no C library (decision 0206, stage 1)

Date: 2026-10-13. Branch of the syscalls agent; the figures below are of
commit 4c22a725 unless a section says otherwise. Hosts: EC2
i-EXAMPLE31 (r8g.4xlarge, Graviton4, linux/arm64, kernel
6.18.54, AL2023, clang 15.0.7, glibc 2.34) and EC2 i-EXAMPLE38
(c7i.4xlarge, Xeon Platinum 8488C, linux/amd64, same software), both on
demand and used only by this agent. xo1 is selfhost/xo built by the Go
backend of the branch (`xo build -o xo1go ./selfhost/xo`), so its
emitters are the branch's selfhost/arm64 and selfhost/llvm.

## What was built

- `rt.sys6(nr, a0..a5)`: a Linux system call, lowered inline by
  selfhost/arm64 and internal/arm64 (a call whose `bl` is `mov x8, x6;
  svc #0`) and by selfhost/llvm and internal/llvm (inline asm, `svc #0`
  with x8 on arm64, `syscall` on x86_64). `rt.sys_restorer()`: the
  address of an rt_sigreturn trampoline the emitters add at its first
  use (rt_sigaction's sa_restorer).
- std/native/llrt/linux.xo: read, write, openat, close, statx (stat,
  lstat, fstat), mmap, munmap, futex, clock_gettime, exit_group,
  getrandom, rt_sigaction, glibc 2.34's strerror table and the C
  runtime's FsErr mapping.
- std/native/llrt/sysio.xo (C path sysio_c.xo, `XO_LLRT_C=sysio`):
  os.fs.read, write, stat (and read_dir's and walk's stats) on Linux.
  Clock.now goes through the native prelude's llrt_clock_now in both
  SSA builders but stays on the C runtime (see "The clock").
- XO_NOLIBC=1 (xo1, Linux targets): the runtime compiled with
  std/llrt/xo_nolibc.c (the C library functions the C runtime calls,
  over raw system calls: entry and TLS, malloc, strings, printf without
  floats, files, directories, epoll, the vDSO clock, signals, futex
  mutexes, clone threads) and linked `-static -nostdlib
  -Wl,--gc-sections`.

## Static programs with no C library

The program of internal/llvm/testdata/golden/sys_files_clock (files and
the clock) and a hello world, built by xo1 with `XO_NOLIBC=1`:

| Host | Backend | `file` | `ldd` | hello world bytes (static / with glibc) |
|---|---|---|---|---|
| linux/arm64 | arm64 | ELF 64-bit LSB executable, ARM aarch64, statically linked | not a dynamic executable | 207,544 / 364,184 |
| linux/arm64 | llvm | ELF 64-bit LSB executable, ARM aarch64, statically linked | not a dynamic executable | 204,248 / 210,968 |
| linux/amd64 | llvm | ELF 64-bit LSB executable, x86-64, statically linked | not a dynamic executable | 45,328 / 37,552 |

Each static binary also ran in an empty root: `sudo chroot DIR /prog`
where DIR holds only the binary, an empty /tmp, and a mounted /proc (the
golden reads /proc/self/status), no libc, no loader. Output identical
to want.txt on all three. (Docker was not running on the Mac; the
chroot on EC2 stands in for a FROM scratch image.)

The golden's matrix (scratchpad gtest.sh), each identical to want.txt:
xo1 dynamic, xo1 `XO_NOLIBC=1`, xo1 `XO_LLRT_C=sysio`, the Go driver
(`XO_SELF_DRIVER=0`): arm64 and llvm on linux/arm64, llvm on
linux/amd64. internal/llvm TestGolden, TestGoldenSplit and
TestGoldenShared of sys_files_clock pass on both hosts; strace on
linux/arm64 (commit 46b4dcd4) shows the reads as Go's os.ReadFile makes them (one statx
of the descriptor, a read of size + 1, a read returning 0).

## Bar: the Xo path against the C path (decision 0180)

bench/perf/sysio/measure.sh: user and kernel instructions (perf stat
`instructions:u`, `instructions:k`) and task-clock milliseconds, minimum
of 5 runs. Workloads: read (os.fs.read of a 4 KiB file, 20,000 times),
read_big (a 1 MiB file, 300 times), write (20,000 writes of 100 bytes),
stat (50,000), clock (500,000 Clock.now). The EC2 hosts ran nothing else;
kernel counts and times still vary by a few percent between runs (write
is kernel bound: its user instructions are the stable figure).

Raw rows (workload, backend, variant, user, kernel, ms):

```
linux/arm64
read arm64 xo 137511590 274963346 50
read arm64 c 204818995 294551521 62
read arm64 nolibc 146891495 270821476 48
read_big arm64 xo 118282846 104699890 19
read_big arm64 c 198996631 113160899 30
read_big arm64 nolibc 118271995 104006607 16
write arm64 xo 134914389 1698832980 617
write arm64 c 135617709 1612611423 646
write arm64 nolibc 139313684 1625724755 637
stat arm64 xo 742241849 262873519 83
stat arm64 c 735945905 258632700 81
stat arm64 nolibc 807956761 260481280 83
clock arm64 xo 46191806 1147612 16
clock arm64 c 46195113 1115084 16
clock arm64 nolibc 41481689 447134 16
read llvm xo 94281328 274893754 46
read llvm c 166709451 294602320 59
read llvm nolibc 100005549 270832384 45
read_big llvm xo 66045803 104658497 16
read_big llvm c 146791952 113207170 28
read_big llvm nolibc 61904864 103990413 13
write llvm xo 90781443 1646770641 639
write llvm c 94705508 1700364728 648
write llvm nolibc 92345663 1715905555 639
stat llvm xo 504932164 262850314 65
stat llvm c 504836952 258590949 65
stat llvm nolibc 543230692 260461659 66
clock llvm xo 40481449 1093783 16
clock llvm c 40485521 1099205 16
clock llvm nolibc 32755662 443023 16
linux/amd64
read llvm xo 86876575 224271048 38
read llvm c 142423495 239832899 50
read llvm nolibc 100732871 223617109 39
read_big llvm xo 73278551 13482944 17
read_big llvm c 75438507 20699837 43
read_big llvm nolibc 92413429 12950836 18
write llvm xo 83636729 1598558854 1128
write llvm c 86219691 1654127438 1108
write llvm nolibc 92753019 1608774377 1330
stat llvm xo 454207509 243041185 60
stat llvm c 450911117 241612226 62
stat llvm nolibc 540058077 242455975 65
clock llvm xo 45556696 1002957 12
clock llvm c 45559658 1016836 12
clock llvm nolibc 41382956 392843 11
```

Ratios (xo over c; nolibc over xo; ms):

| Workload | Backend | xo/c user | xo/c kernel | ms xo/c | nolibc/xo user | nolibc/xo kernel | ms nolibc |
|---|---|---|---|---|---|---|---|
| read | arm64 | 0.671 | 0.933 | 50/62 | 1.068 | 0.985 | 48 |
| read_big | arm64 | 0.594 | 0.925 | 19/30 | 1.000 | 0.993 | 16 |
| write | arm64 | 0.995 | 1.053 | 617/646 | 1.033 | 0.957 | 637 |
| stat | arm64 | 1.009 | 1.016 | 83/81 | 1.089 | 0.991 | 83 |
| clock | arm64 | 1.000 | 1.029 | 16/16 | 0.898 | 0.390 | 16 |
| read | llvm | 0.566 | 0.933 | 46/59 | 1.061 | 0.985 | 45 |
| read_big | llvm | 0.450 | 0.924 | 16/28 | 0.937 | 0.994 | 13 |
| write | llvm | 0.959 | 0.968 | 639/648 | 1.017 | 1.042 | 639 |
| stat | llvm | 1.000 | 1.016 | 65/65 | 1.076 | 0.991 | 66 |
| clock | llvm | 1.000 | 0.995 | 16/16 | 0.809 | 0.405 | 16 |
| read | llvm x86_64 | 0.610 | 0.935 | 38/50 | 1.159 | 0.997 | 39 |
| read_big | llvm x86_64 | 0.971 | 0.651 | 17/43 | 1.261 | 0.961 | 18 |
| write | llvm x86_64 | 0.970 | 0.966 | 1128/1108 | 1.109 | 1.006 | 1330 |
| stat | llvm x86_64 | 1.007 | 1.006 | 60/62 | 1.189 | 0.998 | 65 |
| clock | llvm x86_64 | 1.000 | 0.986 | 12/12 | 0.908 | 0.392 | 11 |

- **The bar holds for files on both backends and both architectures**:
  every xo/c user ratio is at most 1.009 (stat on arm64). Reads gain
  most (0.45x to 0.67x of the C path's user instructions): the C path
  read through stdio (fread in 64 KiB pieces into a growing buffer,
  then a copy into the list), the Xo path reads straight into the
  result. Kernel instructions follow the system calls made (read
  0.93x).
- Clock rows compare the same C call (see "The clock").
- nolibc against glibc: reads and the clock are even or better (the
  clock's kernel ratio, 0.39x to 0.41x, is process start: the `none`
  workload takes 0.44 million kernel instructions static against 1.1
  million with the dynamic loader and glibc); stat is 1.08x to 1.19x and
  amd64 reads 1.16x to 1.26x in user instructions, the shim's byte and
  word loops (memcpy, memset, memcmp, strlen) against glibc's vector
  ones; amd64 write's 1330 ms against 1128 ms is one run in five, its
  user instructions are 1.109x.

## The clock

The first measurement (commit b6b73ff4, linux/arm64) had Clock.now as
`linux.clock_ns(CLOCK_REALTIME)`, the clock_gettime system call from Xo:

```
clock arm64 xo 43191797 263704299 69
clock arm64 c 46195126 1131783 16
clock llvm xo 17981584 263651318 66
clock llvm c 40485521 1099835 16
```

User instructions fell (0.94x, 0.44x) but the system call costs 263.7
million kernel instructions for 500,000 calls against glibc's vDSO's
1.1 million, and 69 ms against 16 ms: **4.3 times the time per call**
(138 ns against 32 ns). Calling the vDSO from Xo needs the vDSO's
address kept between calls (Xo has no module state) and a C ABI call of
an address (an intrinsic). Until then sysio.xo's clock_now calls the C
runtime on every target (its clock_gettime is glibc's vDSO, or with
XO_NOLIBC=1 xo_nolibc.c's own lookup of `__kernel_clock_gettime` /
`__vdso_clock_gettime` in the vDSO's dynamic symbols: 16 ms and 11 ms
above). The same first run had the shim without the vDSO (nolibc clock
245 million kernel instructions, 63 ms), large blocks mapped and
unmapped per malloc (read_big nolibc 1,057,813,657 user and 461,477,206
kernel instructions, 181 ms on arm64) and byte loops for memset; the
fixes are in commits 039a2a34 and fed4a5b3.

## What still needs the C library (stages 2 and 3)

With XO_NOLIBC=1 a program links only what it reaches
(--gc-sections); a C library function that xo_nolibc.c does not
define fails the link and names it. Stage 1 covers what hello world
and the file and clock golden reach. Not covered, as of 4c22a725:

- unwinding: a fault ends the program with its message and status 70
  (fault_exit) without running deferred code and landing pads, and a
  fault in a task cannot be caught (`_Unwind_ForcedUnwind` is a stub);
- floating point text in C (`%e`, `%g`, strtod: the C runtime's Float32
  text and two fault messages): ends the program with a message;
- sockets past shutdown and epoll, and DNS (getaddrinfo is not defined);
- std/http's TLS (Mbed TLS needs the C library; decision 0202 replaces
  it);
- `use c` programs (decision 0205: they link libc by choice);
- the ELF linker of decision 0182 (`--ld=macho`) has no static mode:
  XO_NOLIBC=1 links with cc.

Threads, signals, and the clock are in xo_nolibc.c (clone with a TLS
block per thread, rt_sigaction with its own restorer, the vDSO) but in
C; the Xo runtime's own threads (clone3 from Xo), signal handlers in
Xo, sockets, and the DNS resolver are the ADR's stages 2 and 3.

## Stage 3: sockets and the DNS resolver in Xo

Date: 2026-10-13, netstage3 agent. Final run: commit 69e507f7 on EC2
i-EXAMPLE43 (r8g.8xlarge, Graviton4, linux/arm64, kernel
6.18.54, AL2023, clang 15.0.7, glibc 2.34). linux/amd64: commit
f95c3103 on EC2 i-EXAMPLE09 (c7i, x86_64, same software),
before the resolver's performance work; the amd64 run of the final
commit could not launch (VcpuLimitExceeded) and is left to the lead's
milestone run.

What was built: std/native/llrt/sock.xo (the socket system calls over
rt.sys6), cabi_net.xo and cabi_poll.xo (the exports xo_rt.c calls),
dns.xo (the resolver), net.xo and net_c.xo (`XO_LLRT_C=net`: the C
library's sockets and getaddrinfo). std/llrt/xo_nolibc.c: 1,851 lines
(1,859 before; shutdown, epoll_create1, epoll_ctl, epoll_wait removed,
no socket or resolver function left).

### Bar (decision 0180): bench/perf/net, linux/arm64, commit 69e507f7

Minimum of 5 runs; workload, backend, variant, user instructions,
kernel instructions, ms, checksum:

```
echo arm64 xo 171898879 1275077699 2326 1280000
echo arm64 c 181609061 1272917339 2295 1280000
echo llvm xo 155251435 1273270786 2302 1280000
echo llvm c 165904522 1273552607 2301 1280000
echo llvm nolibc 126139511 1243825396 2289 1280000
dial arm64 xo 26318365 289473004 190 2000
dial arm64 c 28071162 290831046 192 2000
dial llvm xo 23002182 289387104 192 2000
dial llvm c 26099128 288324214 190 2000
dial llvm nolibc 17282434 292205133 190 2000
resolve arm64 xo 50690177 319288936 235 2000
resolve arm64 c 43134810 340059661 241 2000
resolve llvm xo 43329833 315631277 227 2000
resolve llvm c 40685872 339270204 238 2000
```

| Workload | Backend | xo/c user | xo/c kernel |
|---|---|---|---|
| echo | arm64 | 0.947 | 1.002 |
| echo | llvm | 0.936 | 1.000 |
| dial | arm64 | 0.938 | 0.995 |
| dial | llvm | 0.881 | 1.004 |
| resolve | arm64 | 1.175 | 0.939 |
| resolve | llvm | 1.065 | 0.930 |

- Sockets and the poller meet the bar (0.88x to 0.95x user
  instructions). Name resolution of `localhost` (/etc/hosts) misses it
  by 1.065x (llvm) and 1.175x (arm64) in user instructions while making
  fewer kernel instructions (0.93x, 0.94x): /etc/hosts is read and
  scanned on every lookup in Xo, as glibc does, but the scan and the
  address sort allocate. The arm64 nolibc variant does not build on
  main (`__aarch64_ldadd8_*`: libgcc's outline atomics, which stage 2
  provides).
- Before the performance work (commit f95c3103, linux/amd64 LLVM):
  echo 0.938, dial 1.228, resolve 13.2x (433,083,541 against 32,750,467
  user instructions): every lookup parsed nsswitch.conf and built
  strings per line. Commit bd508607 reads a numeric IPv4 host in place,
  keeps the nsswitch.conf order in two words the C owns (keyed by the
  file's statx time and size), scans /etc/hosts as bytes, and sorts with
  no allocation per comparison.
- bench/perf/http (commit f95c3103, 3 runs of 5 s, server CPU per
  request, noisy): linux/arm64 llvm 26.65 µs (Xo) against 27.02 µs
  (XO_LLRT_C=net), arm64 35.37 against 35.31; linux/amd64 llvm 30.34
  against 30.51.

### Correctness

- Golden matrix with xo1 (gmatrix, every golden of
  internal/llvm/testdata/golden): dynamic 153 of 153 on arm64 and llvm
  (linux/arm64, 69e507f7) and llvm (linux/amd64, f95c3103);
  `XO_LLRT_C=net` 13 of 13 network goldens; XO_NOLIBC=1 llvm 131 of 153
  on linux/arm64 (12 fail with exit 70, stage 2's unwinding; 10 do not
  link: std/http's TLS, waitpid, poll), net_echo among the passes.
- In an empty chroot (only the binaries, /tmp, /proc): net_echo,
  conc_select, clock_timers pass static (llvm, linux/arm64 and
  linux/amd64). std/http goldens are still refused: they reference TLS
  runtime symbols even without certificates, so the stub path added in
  selfhost/xo main.xo does not apply to them yet.
- TestResolverDiff (internal/llvm, Go driver, LLVM) passes on
  linux/arm64: two configurations (files then dns with search, ndots:2,
  a CNAME, a truncated answer that needs TCP, NODATA, NXDOMAIN, SERVFAIL;
  dns then files with domain and single-request), the Xo resolver's
  output and the questions its server saw identical to glibc's.
- The resolver program linked with XO_NOLIBC=1 in an empty root holding
  only it and /etc's three files, against glibc's build with the same
  files mounted over /etc, one namespace, the test's DNS server: same
  output, the same 52 questions, on linux/arm64 (llvm) and linux/amd64.
- Real resolution with the hosts' resolv.conf: example.com,
  www.amazon.com, aws.amazon.com, ipv4.google.com, github.com connected
  on port 443, localhost:9 refused, nosuchname.invalid "Name or service
  not known", the same with the Xo path, the C path, and the static
  build, on both architectures.
- TestH2Diff and TestTLSDiff, TestGolden's network goldens (Go
  driver), TestNoGCTests (xo1, dynamic) pass on linux/arm64 and
  linux/amd64. TestSelfhostSSA on the network directories (Mac): 1,822
  and 406 test functions identical between the builders.
