# 0206: Raw system calls on Linux

Status: Accepted (2026-10-13), stage 1 implemented: `rt.sys6` and
`rt.sys_restorer` (std/native/rt.xo; selfhost/arm64 calls.xo, emit.xo,
gen.xo; selfhost/llvm call.xo, gen.xo; mirrored in internal/arm64 and
internal/llvm), std/native/llrt/linux.xo (the system call layer),
std/native/llrt/sysio.xo and sysio_c.xo (files routed, `XO_LLRT_C=sysio`
in both checkers now), std/llrt/xo_sys.c (the layer's buffers),
std/llrt/xo_nolibc.c and selfhost/xo link.xo and main.xo (`XO_NOLIBC=1`).
Tests: golden `sys_files_clock`. Evidence:
`bench/results/linux-syscalls-2026-10-13.md` (`bench/perf/sysio`).
Stage 2 partly implemented (2026-10-13, "Stage 2 as built" below): the
vDSO clock, futex locks and once, signals, thread creation, and the TLS
layout in Xo; unwinding and floating point text remain. Stage 3 is not
started.

## Context

Decision 0203 (owner): Linux programs make raw system calls and link
statically with no C library, as Go does; decision 0195 prerequisite 3
(system calls) comes first in the zero-C audit's order
(bench/results/zero-c-audit-2026-10-13.md, "Design sketches", (3)).
Decision 0205 limits the goal to the runtime and programs without
`use c`; decision 0204 lets the C runtime call Xo. The C runtime
(std/llrt) calls about 90 C library functions; a hello world reaches 47
of them with --gc-sections (threads for the poller and main, epoll,
mutexes, stdio for faults, malloc, getenv).

## Decision

1. **Intrinsics** (internal, decision 0074; no user visible change):
   `rt.sys6(nr, a0, ..., a5) -> Int` returns the kernel's result (a
   negative errno on failure). selfhost/arm64 and internal/arm64 lower
   it as a call whose `bl` is `mov x8, x6; svc #0` (the arguments in x0
   to x5, the number as the seventh argument): the register allocator
   and the call's clobbers are those of a call, which the kernel more
   than honours. The LLVM emitters lower it to inline asm with register
   constraints, `svc #0` with x8 on arm64 and `syscall` (rax, rdi, rsi,
   rdx, r10, r8, r9; rcx and r11 clobbered) on x86_64; selfhost/llvm
   takes the architecture from the driver (`Gen.goarch`), internal/llvm
   from the host (its LLVM builds are the host's). `rt.sys_restorer()`
   is the address of an rt_sigreturn trampoline (number 139 on arm64,
   15 on x86_64) each emitter writes at its first use, for
   rt_sigaction's sa_restorer. The SSA passes do not fold a `match` on
   `target`, so a non-Linux build still lowers the Linux arm (dead
   code); the wasm emitter lowers the layer's intrinsics to a fault.
2. **The layer** is std/native/llrt/linux.xo (one module, not
   `linux/*.xo`: a runtime module path with a slash would need the
   loaders, the std embed patterns, and symbol naming changed): system
   call numbers per `target.arch` (the asm-generic table on arm64,
   x86_64's), read, write and write_all, openat, close, statx for stat,
   lstat and fstat (one structure layout on both architectures),
   clock_gettime, mmap, munmap, futex, exit_group, getrandom,
   rt_sigaction (dispositions; an Xo handler needs a C ABI entry, stage
   2), glibc 2.34's strerror table, and the C runtime's FsErr mapping.
   Structures are Bytes: `rt.bytes_addr` (the data address),
   `rt.bytes_buf` (a fresh buffer), `rt.rs_bytes_len` (a length without
   a copy), `rt.sys_buf` (a per thread 256 byte scratch object with an
   immortal count), read by `rt.bytes_word`: C functions in
   std/llrt/xo_sys.c until decision 0195 prerequisite 2's raw memory
   intrinsics replace them. A buffer whose address a system call uses is
   kept alive past the call (`keep`): reference counting releases a
   value after its last use.
3. **Routing** (decision 0193's mechanism): std/native/llrt/sysio.xo
   gives the prelude os.fs.read (Go's os.ReadFile loop), write
   (os.WriteFile), and stat (also read_dir's, walk's, exists'), with the
   C runtime's semantics and messages, recorded through
   `rt.fs_set_err`. They make the system calls from Xo when the build is
   a Linux one and not a wasm module, and the test world does not fake
   the capability (`rt.tw_fake`); otherwise the C runtime as before.
   `XO_LLRT_C=sysio` loads sysio_c.xo (the C runtime everywhere); xo1's
   checker now reads XO_LLRT_C too (decision 0193 said the self hosted
   compiler had no switch). The Xo path is the default: it meets
   decision 0180's bar (user instructions at most 1.009 of the C path,
   reads 0.45x to 0.67x, on arm64 and LLVM on Graviton and LLVM on
   x86_64). **Clock.now stays on the C runtime**: both SSA builders now
   call the native prelude's llrt_clock_now (sysio.clock_now), but the
   clock_gettime system call from Xo cost 4.3 times the time per call
   of the vDSO (69 ms against 16 ms for 500,000 calls), so it calls
   rt.clock_now until Xo can call the vDSO (next steps). Standard
   streams stay C: the stdout buffer lives in the C runtime, which
   flushes it at exit and before a fault message; moving it needs the C
   runtime to call Xo there (decision 0204's exports).
4. **No C library**: `XO_NOLIBC=1` makes xo1 build a Linux program
   statically with no C library (opt in during stage 1; programs with
   `use c` or std/http's TLS are refused with a message, and it links
   with cc, not `--ld=macho`). The runtime is compiled with
   `-DXO_NOLIBC -ffunction-sections -fdata-sections -fno-stack-protector
   -U_FORTIFY_SOURCE` (and `-mno-outline-atomics` on arm64, also in the
   LLVM IR's target attributes, since libgcc is not linked) and linked
   `-static -nostdlib -Wl,--gc-sections`. std/llrt/xo_nolibc.c, included
   at the end of xo_rt.c, defines the C library functions the C runtime
   calls over raw system calls: `_start` (arguments, environment,
   auxiliary vector, the static TLS block and thread pointer, main on a
   1 GiB stack), malloc and friends over mmap, strings, integer parsing,
   qsort, printf without floating point, files, directories
   (getdents64), epoll, the vDSO's clock_gettime, getrandom, signals
   (its own restorer), futex mutexes and once, and threads (clone with a
   stack and TLS block, joined through CLONE_CHILD_CLEARTID). It is C:
   scaffolding for the C runtime that shrinks as the runtime moves to
   Xo, not a second libc for Xo code (which uses linux.xo). A hello
   world and the sys_files_clock golden link and run with no libc on
   linux/arm64 (arm64 and LLVM backends) and linux/amd64 (LLVM),
   including in an empty chroot.

### Stages

- **Stage 2** (the runtime's own threads, signals, unwinding):
  - Threads from Xo: clone3 (`CLONE_VM | FS | FILES | SIGHAND | THREAD
    | SYSVSEM | SETTLS | PARENT_SETTID | CHILD_CLEARTID`) with a stack
    and TLS block mapped by linux.xo, the child entering a C ABI entry
    (`rt.c_entry_addr`, the audit's sketch) that sets up the Xo thread
    state; joins on the cleared tid with futex. Needs: C ABI entries in
    the emitters, raw memory (prerequisite 2) for the TLS image, and
    the runtime's thread state (prerequisite 6). Until then xo_nolibc.c's
    pthread_create does it in C.
  - TLS setup from Xo: the PT_TLS image copied into each thread's block
    (the layout xo_nolibc.c's nl_tls_init uses: variant I on arm64,
    variant II with the self pointer on x86_64); selfhost/macho's ELF
    linker relaxes TLS sequences to local exec already.
  - Signals in Xo: rt_sigaction with rt.sys_restorer (done), handlers
    as C ABI entries, sigaltstack for stack overflow faults.
  - Unwinding without libgcc: a DWARF CFI unwinder over .eh_frame (and
    .eh_frame_hdr's table) in Xo, driving the existing personality
    routine, or the frame walk xo_rt.c does on Windows over
    RtlVirtualUnwind done over .eh_frame; until then faults end the
    program (fault_exit) without deferred code.
  - The vDSO from Xo: the vDSO's address (AT_SYSINFO_EHDR, auxv) and
    its symbol table read with raw memory, the clock function's address
    kept in runtime state, and an intrinsic for a C ABI call of an
    address (`rt.call_c2(f, a, b)`: `blr` and an LLVM indirect call);
    then sysio.clock_now's Linux arm is linux.clock_ns.
  - Floating point text without the C library: the C runtime's Float32
    and fault message paths call the Xo text (num.xo) through decision
    0204's exports instead of `%e` and strtod.
  - The ELF linker (decision 0182) writes static executables with
    section GC, so XO_NOLIBC=1 needs no cc for arm64 (the runtime
    object from a nolibc pack variant, decision 0157).
- **Stage 3** (the network):
  - **Implemented 2026-10-13** (bench/results/linux-syscalls-2026-10-13.md,
    "Stage 3"). Sockets from Xo: std/native/llrt/sock.xo makes socket,
    bind, listen, accept4, connect, getsockopt (SO_ERROR) and
    setsockopt, getsockname and getpeername, sendto (MSG_NOSIGNAL) and
    recvfrom, read, shutdown, close, fcntl, ioctl, epoll_create1,
    epoll_ctl, epoll_pwait, and ppoll through rt.sys6. The C runtime
    keeps its structure (the poller thread, poll descriptors, handles,
    messages) and calls them through C ABI exports (decision 0204):
    std/native/llrt/cabi_net.xo (sockets and the resolver; built for a
    program that dials or listens, the SSA builders' uses_net, or that
    uses std/http, whose implementation imports it) and cabi_poll.xo
    (epoll, close, nonblocking, shutdown: what the poller reaches in
    every program, so built in every native program;
    gotchas/runtime-export-reached-without-feature.md). The C declares
    cabi_net's exports weak and asks net_raw() once: without them, or
    with `XO_LLRT_C=net` (net_c.xo, decision 0193's mechanism), it calls
    the C library as before. With XO_NOLIBC the references are strong
    and there is no C path (xo1 refuses `XO_LLRT_C=net`).
  - The DNS resolver in Xo: std/native/llrt/dns.xo does getaddrinfo's
    job for listen and dial, following Go's pure Go resolver
    (nsswitch.conf's files and dns, /etc/hosts, resolv.conf's
    nameserver, domain, search, ndots, timeout, attempts, rotate,
    single-request, use-vc, trust-ad; A and AAAA in parallel over UDP
    with EDNS0, TCP on truncation; RFC 6724 ordering) with glibc's error
    codes and texts; its differences from glibc are listed at the top of
    dns.xo. A numeric IPv4 host is read in place; the nsswitch.conf
    order is kept between lookups in two words the C owns, keyed by the
    file's statx time and size. inet_ntop and inet_pton(AF_INET6) of
    the HTTP runtime are Xo too. Tested against glibc with a local
    server in a namespace (internal/llvm TestResolverDiff,
    gotchas/resolver-test-needs-port-53.md) and against real
    resolution on EC2.
  - xo_nolibc.c lost shutdown and the epoll functions; no socket or
    resolver function is left in it.
  - Bar: sockets and the poller 0.88x to 0.95x the C path's user
    instructions; resolving localhost 1.065x (LLVM) and 1.175x (arm64),
    with 0.93x the kernel instructions (bench/perf/net, linux/arm64).
  - XO_NOLIBC=1 would link std/http programs that reference no TLS
    runtime symbol with the runtime's TLS stubs; every std/http golden
    references some (the client's default roots), so they are still
    refused until decision 0202.
  - Left: the resolve bar (an allocation free hosts scan and parse); TLS
    in Xo (decision 0202) for std/http without Mbed TLS; the
    poll descriptors and the poller thread move to Xo with stage 2's
    threads; caching /etc/hosts (Go keeps it 5 seconds) needs runtime
    state for a list, which the C's two words cannot hold.
- **Default**: XO_NOLIBC=1 becomes the default for Linux builds
  without `use c` when every golden passes with it on both backends
  and both architectures (stage 3), and decision 0180's bar holds for
  bench/perf/sysio's nolibc variant (stat and amd64 reads miss it in
  stage 1: 1.08x to 1.26x user instructions, the shim's memcpy and
  friends against glibc's).

### Stage 2 as built (2026-10-13)

Evidence: `bench/results/linux-syscalls-2026-10-13.md`, "Stage 2".

- **Intrinsics** (internal): `rt.call_c2(f, a, b)` calls the C function
  at address f (`blr`; an LLVM indirect call); `rt.sys_words()` is the
  address of 16 zeroed words of process state each native emitter
  writes at its first use (a local data object on arm64, a weak
  `@.xo.sys_words` on LLVM, one per split build); `rt.sys_clone(flags,
  sp, tid, tls)` makes clone(2) through a trampoline the selfhost
  emitters write (the child loads a C function and its argument from
  its stack, calls it, exits the thread; the Go emitters do not have it:
  only XO_NOLIBC builds, which the Go driver does not make, reach it).
  C accessors until the emitters address thread local storage:
  `rt.auxval` (getauxval) and `rt.sys_clock_buf` (the thread's timespec
  scratch, 0 under the test world's virtual clock and off Linux; the
  wasm emitter lowers it to 0). `rt.clock_virtual` is gone.
- **The vDSO**: linux.xo finds `__kernel_clock_gettime` /
  `__vdso_clock_gettime` in the vDSO's dynamic symbols and keeps its
  address in word 0 of `rt.sys_words`; Clock.now on Linux calls it from
  Xo (sysio.clock_now, one C call on the path, no `target` test, which
  the arm64 emitter would not fold). Against the C path (7b4d0755):
  0.967 of the user instructions on the arm64 backend, 0.864 on LLVM
  arm64, 0.956 on LLVM x86_64, within decision 0180's bar. `XO_LLRT_C=sysio` keeps the C clock. The Go
  backend's assembly mode keeps the C clock (no `target` there).
- **The static runtime's clock, locks, signals, threads**:
  std/native/llrt/cabi_nolibc.xo, exports (decisions 0204, 0207) that
  the SSA builders build only for an XO_NOLIBC build
  (`ssa.build_program_libc`; the Go builder skips the module):
  clock_gettime, the futex mutex (Drepper's, on the first word of
  pthread_mutex_t, and malloc's lock), pthread_once (32 bits: its 0 to 1
  step under a once lock in `rt.sys_words`), sigaction, signal,
  sigprocmask (the kernel's structure with `rt.sys_restorer`), the TLS
  block's size and layout from AT_PHDR's PT_TLS (also for the main
  thread: C sets the thread pointer only), thread_create and
  thread_join. xo_nolibc.c keeps a one line forward per C library name:
  1,859 lines to 1,622. The C runtime with glibc (no XO_NOLIBC) is
  unchanged.
- **XO_NOLIBC arm64 links** write libgcc's outline atomics themselves
  (the arm64 emitter calls them; `macho.outline_atomic`).
- Still C in xo_nolibc.c: entry and the thread pointer, malloc, strings,
  printf, files, directories, epoll, getrandom, and unwinding stubs
  (faults in static programs end the program without deferred code: 12
  goldens). Next: an unwinder over .eh_frame in Xo, the remaining system
  call wrappers forwarded to linux.xo, and thread local storage from the
  emitters (replacing `rt.sys_buf` and `rt.sys_clock_buf`).

## Consequences

- Linux builds of std/native code now contain inline system calls; the
  C library is out of the file paths of every Linux program, and out of
  the whole process with XO_NOLIBC=1.
- A wasm build keeps the host's `target.os` (a pre-existing behaviour,
  gotchas/wasm-target-os-is-the-host.md); xo1 now gives wasm builds the
  wasm architecture, which sysio.xo's guard tests, and the wasm emitter
  turns the layer's intrinsics into faults.
- The C shim adds 1,851 lines of C (std/llrt/xo_nolibc.c, 2026-10-13,
  after stage 3; 1,859 in stage 1)
  under `#ifdef XO_NOLIBC`, compiled only into XO_NOLIBC builds; each later port removes some of its
  callers.

## Alternatives

- A third party libc subset (musl, the kernel's tools/include/nolibc):
  more C and an outside dependency, against decision 0195's direction.
- Waiting for the whole runtime to move to Xo before linking without
  libc: blocks decision 0203 behind every other prerequisite.
- C runtime functions calling linux.xo through decision 0204's exports
  for their system calls: the right end state for the shim's system
  call wrappers, but no export mechanism exists yet; next step.
- The clock through the system call now (as planned): rejected by the
  measurement (4.3 times the time per call).
