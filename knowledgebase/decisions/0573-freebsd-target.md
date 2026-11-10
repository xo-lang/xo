# 0573: FreeBSD as a target (freebsd/amd64 and freebsd/arm64)

Status: Accepted (owner, 2026-11-08, on the lead's recommendation). Proposed (Xo Lang 1, 2026-11-08; the owner decides). The lead
queued a FreeBSD ADR on 2026-11-08 behind linux/riscv64 (decision 0571).
Stage 1's cross build landed behind the target on 2026-11-08
(Progress below); the status stays Proposed. Go's FreeBSD port (amd64 and
arm64, both first class in Go) is the reference for the system calls,
structures and runtime glue.

## Context

Xo's runtime talks to the operating system in two ways today
(main 3df16d62d):

- **Linux: raw system calls** (decision 0206): std/native/llrt/linux.xo
  issues them through `rt.sys6` (`svc #0`, `syscall`), with the
  asm-generic or x86_64 numbers (`nr(arm64, amd64)`), and a result of
  zero or more, or a negative errno.
- **Darwin: libSystem** (std/native/sys_darwin.xo and llrt darwin.xo),
  since Apple supports no system call ABI; the same API (a result of zero
  or more, or -errno) over C calls.

`sysio.xo` and the other OS layers choose one by `match target.os`.
`TargetOs` is `Linux Darwin Windows Wasi` (spec 12.2); 48 files under
std/ and compiler/ test `TargetOs.Linux` and 47 `TargetOs.Darwin` (43 of
them in llrt), 463 references to `TargetOs` in all. Each exhaustive match
needs a FreeBSD arm.

FreeBSD differs from Linux where the runtime is lowest:

| Part | Linux (today) | FreeBSD | Go's reference |
|---|---|---|---|
| System call ABI | `svc #0` / `syscall`, -errno in x0 / rax | the same instructions; the number in x8 / rax; failure sets the carry flag and returns a positive errno | sys_freebsd_amd64.s, sys_freebsd_arm64.s |
| System call numbers | asm-generic (arm64), x86_64 table | one table for every architecture (syscalls.master) | syscall/zsysnum_freebsd_*.go |
| Poller | epoll | kqueue; `struct kevent` is 64 bytes since FreeBSD 12 (the `ext[4]` words), darwin's 32 | runtime/netpoll_kqueue.go, defs_freebsd_*.go |
| Futex | futex(2) | `_umtx_op` with `UMTX_OP_WAIT_UINT_PRIVATE` and `UMTX_OP_WAKE_PRIVATE` | runtime/os_freebsd.go futexsleep, futexwakeup |
| Threads | clone(2) through a raw stub | `thr_new(struct thr_param)`, `thr_self`, `thr_kill` | os_freebsd.go newosproc, sys_freebsd_*.s thr_start |
| Thread pointer | fs base (amd64), tpidr_el0 (arm64) | the same registers; amd64 sets fs base with `sysarch(AMD64_SET_FSBASE)` | sys_freebsd_amd64.s settls |
| Signals | rt_sigaction, Linux `ucontext` | `sigaction` with FreeBSD's `struct sigaction`, `ucontext_t` and `mcontext_t` layouts | defs_freebsd_*.go, signal_freebsd_*.go |
| Executables | ELF, static or dynamic | ELF with the FreeBSD ABI note (`NT_FREEBSD_ABI_TAG` in `.note.tag`) and, when dynamic, `/libexec/ld-elf.so.1` | cmd/link elf.go |
| Time | vDSO `__vdso_clock_gettime` | `clock_gettime` (the shared page's fast path is optional) | sys_freebsd_*.s |

FreeBSD keeps its system call ABI stable across releases (the COMPAT
options), which is why Go makes raw system calls there, as on Linux, and
links no libc by default; OpenBSD and macOS are the BSDs where it must
not.

## Decision (proposed)

1. **Targets.** `freebsd/amd64` and `freebsd/arm64`, FreeBSD 14 and
   later (13 reached end of life on 2026-04-30), with `TargetOs.FreeBSD`
   (spec 12.1 and 12.2; user code with an exhaustive `match target.os`
   adds an arm). Neither is in `--targets=all` until stage 4 below.
2. **Raw system calls, as on Linux.** A new llrt `freebsd.xo` with the
   API of linux.xo (a result of zero or more, or -errno), so `sysio.xo`
   and the layers above it take the FreeBSD arm with no change in shape.
   The system call stub turns FreeBSD's carry flag and positive errno
   into -errno in one place (an `rt.sys6` variant per OS in the native
   and LLVM backends). No libc: static binaries, as Go's default.
3. **Native backends first.** compiler/arm64 already emits arm64 and
   x86-64 ELF objects and compiler/macho's elflink links them; FreeBSD
   needs the ABI note, its system call stub, its TLS setup and its
   signal frame offsets, not a new instruction selector. The LLVM
   backend follows with the `x86_64-unknown-freebsd14` and
   `aarch64-unknown-freebsd14` triples, which needs a FreeBSD sysroot
   only if a program links C.
4. **Layer by layer, with Go's file as the reference:** the poller on
   kqueue (poll.xo's darwin kqueue path, with FreeBSD's 64-byte
   `kevent`), the futex on `_umtx_op`, threads on `thr_new`, signals
   and fault frames on FreeBSD's `ucontext_t`, the profiler's registers
   (prof.xo's per OS offsets), and `kern.hostname`, `kern.arandom` and
   the other sysctls where Linux reads /proc or getrandom (FreeBSD 12
   has `getrandom(2)` too).
5. **Testing.** FreeBSD 14 virtual machines: arm64 under qemu-system
   with KVM on offload host 2 if nested virtualization is available,
   else on the owner's Mac (UTM, Apple's hypervisor); amd64 under
   bhyve or qemu on the linux/amd64 offload host. Cross builds from the
   Mac, binaries copied in, the native goldens run there; a FreeBSD
   lane in the full tier once the goldens pass.

## Plan

1. **Stage 1: hello world.** `TargetOs.FreeBSD` (spec, prelude, the
   target fold, every exhaustive match with a FreeBSD arm, most as
   Linux's arm or a fault), the ABI note in elflink, the system call stub
   and numbers, write and exit, TLS setup, a static freebsd/amd64 and
   freebsd/arm64 hello world that runs in the VM.
2. **Stage 2: the runtime.** Threads, the futex, the poller, signals and
   faults, time, the file system calls; the native goldens that need no
   network, with a list of the rest and why.
3. **Stage 3: network and tools.** Sockets, DNS (`/etc/resolv.conf`, as
   Linux), TLS, HTTP; `xo test` of std; XO_PROF and the fault traceback
   on FreeBSD's frames.
4. **Stage 4: in `all`.** Every native golden in the VM, both targets
   added to `--targets=all`, release archives; the LLVM backend's triples.

## Consequences

- Two more targets to keep green, each needing a virtual machine in the
  test lanes; stage 1 can land without one (cross builds only), stage 2
  cannot.
- `TargetOs` grows (spec 12 is [P1]): an exhaustive `match target.os` in
  user code needs an arm, listed in "Changes in 0.N".
- The OS layer's split (linux.xo, darwin.xo) becomes three; most of the
  463 `TargetOs` references take Linux's arm unchanged (paths, /etc
  files, the errno names), so the cost is in the 43 llrt files, not std's
  surface.
- FreeBSD's errno values are BSD's, not Linux's (`EAGAIN` 35 against
  11, `EINPROGRESS` 36 against 115); darwin's are BSD's too, so the
  runtime's errno constants take darwin's values for the common ones,
  checked against FreeBSD's errno.h.

## Alternatives

- **libc instead of raw system calls**, as on darwin: works and avoids
  the carry flag stub, but adds a dynamic link and FreeBSD's libc
  startup to every binary, against decision 0195 (no compiled C
  runtime) and Go's choice for FreeBSD. Kept for a program that links C.
- **The Linux binary under FreeBSD's Linuxulator** (linux64.ko): runs
  many Linux binaries unchanged, but needs the module loaded, emulates
  epoll and futex with gaps, and is not what a FreeBSD user expects of
  a target.
- **The Go backend only** (`--backend=go`, Go's freebsd ports): available
  today for what the Go backend builds, but it does not run Xo's
  runtime; it stays available.
- **One target first** (amd64 only): FreeBSD's servers are mostly
  amd64; the arm64 port shares everything but the stub, TLS and signal
  offsets, so the plan builds both and may ship amd64 first if arm64's
  VM lane lags.

## Progress

- **2026-11-08, stage 1, cross build (Xo Lang 1; the lead: cross build
  only until a VM lane exists).** `TargetOs.FreeBSD` in the prelude and
  every exhaustive `match target.os` of compiler/ and std/: the
  compiler's arms (the target fold, host paths, the backend map:
  freebsd/arm64 to the arm64 backend, freebsd/amd64 to amd64), and std's
  117 runtime arms join Wasi's (a stub that reports "no" or does
  nothing) except write, exit and the heap's map and unmap, which call
  the new `std/native/llrt/freebsd.xo`. The driver treats freebsd as an
  ELF OS (`elf_os`): Xo's ELF linker, no C library (`nolibc` is always
  true; a `use c` program does not build), the nolibc `_start` and the
  ELF sections of the native backends. The link brands the executable
  with EI_OSABI 9 (ELFOSABI_FREEBSD), which FreeBSD's kernel accepts for
  a static executable; the `.note.tag` ABI note is left for stage 2 if
  the VM shows it is needed. `rt.sys6` adds FreeBSD's error convention
  after the trap: `cneg x0, x0, cs` on arm64, `sbbq %rcx, %rcx; xorq
  %rcx, %rax; subq %rcx, %rax` on amd64 (rcx is clobbered by `syscall`),
  so every caller keeps Linux's negative errno. The main thread's TLS
  block is mapped with FreeBSD's flags and amd64 sets its fs base with
  sysarch(AMD64_SET_FSBASE). A hello world builds for freebsd/amd64 and
  freebsd/arm64 from a Mac: ELF64, EI_OSABI 9, static, `_start` entry, a
  PT_TLS segment (llvm-objdump). Nothing runs yet: threads (the main
  runs on a thread of its own), the futex, the poller, signals, time and
  the file system are stage 2, which needs the VM.
  The goldens and bench programs with an exhaustive `match target.os`
  put FreeBSD in Linux's arm (method_values, fs_paths_os,
  sys_files_clock, gen's audit_second_pass, bench/xo/v0.4 and the
  planted variants of 04-file-sync; the amd64 sample's method_values
  found it); bench/paper's blind solutions are left as written, so they
  no longer check, as user code of their age would not.
- **2026-11-08, stage 2 part 1: what the host can check (Xo Lang 1; the
  lead: cross built code with host unit tests only, kernel-only behavior
  parked for the VM lane).** `sysio.layer()` is 4 on FreeBSD (it was 0,
  the wasm runtime's layer, so the 52 `layer() == N` sites took WASI's
  paths), in llrt and the SSA builder's fold. freebsd.xo gains read,
  write, open (openat), close, fsync, stat (fstatat), fstat, the clock
  (clock_gettime 232, the system call: AT_TIMEKEEP's shared page later),
  nanosleep, and wrappers for _umtx_op, thr_new, thr_self, thr_exit and
  thr_kill, with the struct offsets as named constants; sysio's file
  calls and clock use them, and errno messages and FsErr come from the
  BSD table (darwin's). EAGAIN is 35 in the HTTP and TLS retry tests,
  AF_INET6 is 28, and a sockaddr's family is its second byte, as on
  macOS. Checked on the host: `TestFreeBSDABI` (internal/repotest)
  compares every constant and system call number of freebsd.xo with
  testdata/freebsd-abi.txt, which scripts/freebsdabi/gen.sh computes with
  clang for x86_64 and aarch64 from the 14.5-RELEASE src.txz (sha256
  267fb92d..., its MANIFEST's); a wrong offset fails it (checked by
  mutation). Parked for the VM: the threads on thr_new, the futex on
  _umtx_op (the wrappers exist, nothing calls them), the kqueue poller,
  signals and faults, and statfs.

- 2026-11-09, the first VM runs (offload.md "FreeBSD VMs under QEMU"):
  a hello on freebsd/amd64 reached the poller's Linux pipe2 (SIGSYS),
  then, with the poller skipped, Linux's mmap (FreeBSD's link) from
  libcthr's thread_create and a fault 0x1050 below the TLS block. The
  cause of the fault: `_start` read the initial stack from rsp (sp),
  but FreeBSD passes it in rdi (x0) and starts rsp 8 bytes below it
  when it is 16 byte aligned (sp rounded down on arm64; exec_setregs),
  so argc was off by a word, the auxiliary vector was missed, and the
  TLS block was mapped for no PT_TLS (96 bytes, not 224). Fixed in
  ssa.entry_stub_amd64 and entry_stub_arm64; until stage 2's threads
  and kqueue poller land, entry.xo's signals() takes the no-poller path
  on FreeBSD and thread_create fails (main runs on the first thread),
  and unwind.xo's scratch maps with FreeBSD's mmap. VM confirmation of
  this step waits for the amd64 host.
- 2026-11-09, stage 2 written (unpushed until a VM run proves it):
  threads on thr_new (libcthr fb_thread_create: clone's mapping, the
  thread pointer as tls_base, cabi_nolibc fb_thread runs the function
  then thr_exit, which sets the record's long to 1 and wakes it; join
  waits with _umtx_op UMTX_OP_WAIT on that long), the kqueue poller
  (poll.xo's FreeBSD arms: pipe2 with O_CLOEXEC | O_NONBLOCK, kqueue,
  64 byte struct kevent through KEV_*, ev_bytes for the event buffers),
  and SIGINT, SIGTERM, SIGQUIT and SIGPIPE through sigaction 416
  (entry.xo freebsd_sig; the kernel supplies the return trampoline).
  scripts/freebsdabi/gen.sh gains O_NONBLOCK, F_SETFD, F_GETFL, F_SETFL
  and FD_CLOEXEC; TestFreeBSDABI covers the new wrappers.
- 2026-11-09, stage 2 run on a FreeBSD 14 amd64 VM (Xo Lang 1): 119 of the
  120 goldens the harness runs pass (36 at the first run). Beyond threads,
  the poller and signals it took: heapos.map_aligned and fiber stacks on
  FreeBSD's mmap; pool layer 4 (umtx futex waits); getrandom 563; exit(2)
  for exit_group; fsops layer 4 (*at calls, getdirentries); sock.xo giving
  its callers Linux's socket ABI (errnos, sockaddrs, flags) on FreeBSD, so
  hclient, hconn and the TLS loops' would_block and peer family take
  Linux's arm; netpoll on; the host name by sysctl kern.hostname; the
  auxiliary vector kept at start (the unwinder's .eh_frame_hdr: without it
  every fault escaped isolate and defer); and linux.xo's shared wrappers
  (open with its flags mapped, read, write, close, fsync, stat, fstat,
  clock_gettime with its clock ids) taking FreeBSD's calls, since code
  with a Linux arm reaches them there
  (gotchas/freebsd-sock-linux-errnos.md). The one left, fs_ops_edges,
  differed because the VM ran it as root (permission checks pass); run
  as nobody in a directory it owns, it passes too: 120 of 120 at
  2947ad02e (2026-11-10). Not run yet: arm64 FreeBSD, and the harness's
  other suites.
- 2026-11-10, every LLVM golden in FreeBSD 14.5 VMs (the tier script
  scripts/lead/freebsd-tier.sh: cross built on the Mac with the native
  backend, run in a VM, stdout compared exactly): 642 of 642 on
  freebsd/arm64 (QEMU with hvf on the Mac) and on freebsd/amd64 (KVM on
  the amd64 host); four goldens need `use c` or the Linux-only C
  runtime and do not build. Stage 3's fault reports and profiles came
  with it: crash.xo's handlers through FreeBSD's sigaction (416), stack_t
  and siginfo; unwind.xo's mcontext registers and the ELF unwind tables
  (profiles and fault frames, std/debug's /profile answered 503 before);
  prof.xo's SIGPROF and setitimer. The rest: Proc.pid/ppid/usage,
  Fs.disk_usage (statfs 555), Net.interfaces (NET_RT_IFLIST),
  unlinkat 503, a failed exec's exit and signal reset, and an audit of
  linux.xo's calls with Linux-only numbers: same-argument calls take
  FreeBSD's numbers, the others return ENOSYS rather than make another
  call (gotchas/freebsd-linux-syscall-numbers.md; dns_records, which
  faulted on freebsd/amd64 before the audit, passes after it). FreeBSD reports a child's
  max_rss as 0 when it exits before its first clock tick (spec/std.md).
  Left: asynchronous preemption (loops poll; design in the lead's
  notes), `use c`'s libc numbers.
- 2026-11-10 (Xo Lang 1): asynchronous preemption on freebsd/arm64 and
  freebsd/amd64: SIGURG through sigaction 416 to the worker thread
  (thr_self 432, thr_kill 433), the interrupted pc and sp from the
  ucontext's mcontext at 16 (arm64 sp 248, elr 256; amd64 rip 160, rsp
  184), the mask through sigprocmask 340; compiler/ssa/build.xo turns it
  on for freebsd. Proved in the FreeBSD 14.5 VMs with
  async_preempt_regs and async_preempt_alloc under a probe that counts
  each signal past the pc and sp check 1,000 more times: arm64 6,006 and
  21,021 (6 and 21 preemptions, every one through the signal), amd64
  6,006 and 61,061 (6 and 61), output equal to want.txt. Left: `use c`'s libc numbers.
- 2026-11-10 (Xo Lang 1): `use c`, stage 1: a binding's C (its own
  sources and the generated adapter) is compiled by clang for
  `<arch>-unknown-freebsd14.5`, freestanding, against the headers of an
  unpacked base.txz (`XO_FREEBSDSYSROOT`, scripts/freebsd-sysroot.sh), and
  Xo's ELF linker links it with no C library; `link` libraries are an
  error. use_c_add, use_c_bytes and use_c_enum (C with no C library calls;
  the adapter's unused malloc helper drops out at -O2) pass in the FreeBSD
  14.5 VMs: arm64 3 of 3, amd64 3 of 3. C calling malloc, free, memcpy
  and strlen links already (std/native/llrt/cabi_libc.xo's C names in a
  build with no C library); calloc and snprintf do not.
- 2026-11-10 (Xo Lang 1): `use c`, stage 2: std/native/llrt/cabi_cbind.xo
  exports malloc, calloc, realloc, memchr, strchr, strrchr, strcmp,
  strncmp, strspn, strcspn, strtol, atoi, atol and atoll under their C
  names, only in a FreeBSD program with `use c` (compiler/ssa
  declare_exports). Xo's ELF linker now names every missing function of
  a static link at once, and a strong reference wins over a weak one
  (the runtime's weak malloc made the adapter's malloc call a nop,
  gotchas/elflink-weak-before-strong.md). use_c_libc (Str arguments
  through the adapter's malloc copy, the string functions, calloc and
  realloc, strtol's prefixes, saturation and end pointer) passes in both
  VMs. Left: printf's family (C varargs) and `<ctype.h>` (FreeBSD's
  locale tables).
