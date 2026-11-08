# 0573: FreeBSD as a target (freebsd/amd64 and freebsd/arm64)

Status: Proposed (Xo Lang 1, 2026-11-08; the owner decides). The lead
queued a FreeBSD ADR on 2026-11-08 behind linux/riscv64 (decision 0571).
This ADR is the plan; nothing is built yet. Go's FreeBSD port (amd64 and
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
