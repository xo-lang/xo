# 0197: Windows on the LLVM backend

Status: Accepted (2026-10-12, windows agent, on the lead's task for
decision 0196); stage 1 implemented for windows/amd64. Later stages
Proposed.

## Context

Decision 0196 gates the Go compiler's deletion on the LLVM backend
building Windows programs (windows/amd64, then windows/arm64) that pass
the golden suite. Before this decision the LLVM backend built for the
host only, on darwin and Linux, and its runtime (std/llrt, one C
translation unit: xo_rt.c includes the rest) was POSIX: pthreads, a
futex, kqueue or epoll, sockets, signals, `posix_spawn`, termios, and
faults raised with `_Unwind_ForcedUnwind`.

Hosts: an on demand EC2 Windows Server 2025 instance (c7i.4xlarge, 16
vCPU, owner approved about $1.40 an hour), set up by
`scripts/remote-windows.sh` with LLVM 23.1.3 (clang, lld-link), the MSVC
build tools and Windows SDK (VS 2022 Build Tools, VCTools workload), Go
1.26.1, and Git for Windows. `xo` itself is the Go driver built on the
instance (`go build ./cmd/xo`); native builds do not run xo1 there
(selfdriver.Wants is arm64 only).

## Inventory: what the native runtime needs from the OS

| Need | Where (std/llrt) | POSIX today | Windows equivalent | Stage 1 |
|---|---|---|---|---|
| Threads (worker pool, main's 1 GiB stack, poller) | xo_rt.c | pthread_create/join/once, mutexes | `_beginthreadex` with `STACK_SIZE_PARAM_IS_A_RESERVATION`, SRW locks, `InitOnceExecuteOnce`, condition variables (TLS library) | done (xo_win.c) |
| Futex (xo_lock, parks, worker wake) | xo_rt.c | `__ulock_*` (macOS), `SYS_futex` | `WaitOnAddress`, `WakeByAddressSingle` | done |
| Fiber switch (tasks) | xo_rt.c | own asm, SysV callee saved | own asm with the Windows x64 callee saved set (rbx, rbp, rdi, rsi, r12 to r15, xmm6 to xmm15, MXCSR, x87 CW) and the TEB stack bounds (StackBase gs:0x08, StackLimit gs:0x10, DeallocationStack gs:0x1478) | done |
| Fiber stacks | xo_rt.c | mmap + guard page | `VirtualAlloc` reserve, 64 KiB committed, `PAGE_GUARD` below (the system grows it as a thread stack) | done |
| Fault unwinding (decision 0085) | xo_rt.c | `_Unwind_ForcedUnwind` + stop function, `xo_personality` | the runtime walks frames itself: `RtlCaptureContext`, `RtlLookupFunctionEntry`, `RtlVirtualUnwind`, finds the nearest frame whose language handler is `xo_personality` and whose GCC call site table (LLVM emits it in .xdata for a non MSVC personality) has a landing pad, `RtlRestoreContext` there; `xo_personality` itself is the SEH handler and passes system exceptions on | done |
| Hardware faults (stack overflow, access violation) | none today on POSIX either | | a vectored exception handler that reports `fault: stack overflow` | not yet |
| Timers and wake ups (poller) | xo_rt.c | kqueue/epoll + self pipe | `WSAPoll` with a loopback socket pair as the self pipe | done |
| Socket readiness | xo_rt.c | kqueue/epoll, edge triggered | `WSAPoll`, level triggered: a descriptor is polled only in the directions someone waits in (waiters poke the poller); IOCP with AFD polling (as wepoll and mio) for scale later | done (WSAPoll) |
| Sockets | xo_rt.c, xo_http.c, xo_h2.c, xo_tls.c | BSD sockets, errno | Winsock 2 (`WSASocketW` not inherited), WSA errors mapped to errno, `WSAEWOULDBLOCK` connect as `EINPROGRESS`, AF_UNIX (afunix.h); input drained before shutdown (Windows resets a connection shut for receiving with unread input) | done |
| Files by path (Fs) | xo_fs.c, xo_bytes.c, xo_time.c | open, stat, opendir, rename, utimensat, flock, symlink | UTF-8 paths to UTF-16: `_wopen` (binary, not inherited), `CreateFileW` + `GetFileInformationByHandle`, `FindFirstFileExW`, `MoveFileExW`, `SetFileTime`, `LockFileEx`, `CreateSymbolicLinkW`, `FSCTL_GET_REPARSE_POINT`; Win32 errors classified as POSIX reports them (a file parent is ENOTDIR, an open of a directory EISDIR) | done |
| Permission bits | xo_fs.c | chmod/stat modes | only the read-only attribute (0666 or 0444, directories 0777), as Go's os on Windows | done (target dependent golden) |
| Standard input | xo_stdin.c | poll on fd 0 via the poller | blocking `_read` (WSAPoll takes sockets only); a reader thread later | done, blocking |
| Console | xo_rt.c, xo_stdin.c | write(1), termios, TIOCGWINSZ | `_setmode(_O_BINARY)` on 0 to 2, `SetConsoleOutputCP(CP_UTF8)`, `GetConsoleScreenBufferInfo`, `SetConsoleMode` (echo) | done |
| Arguments and environment | xo_rt.c | argv, environ | `CommandLineToArgvW` converted to UTF-8; `_environ` (ANSI) | args done; environment UTF-8 later |
| Signals and shutdown (decision 0042) | xo_rt.c | SIGINT, SIGTERM to the self pipe | `SetConsoleCtrlHandler` (Ctrl-C, Ctrl-Break, close, shutdown) wakes the poller | done |
| Clocks | xo_rt.c | clock_gettime | `QueryPerformanceCounter`, `GetSystemTimePreciseAsFileTime` | done |
| Time zones | xo_time.c | zone data compiled in (XO_TIME) or /usr/share/zoneinfo, /etc/localtime | the compiled in data works; the local zone is "Local" (UTC) until the registry's `TimeZoneKeyName` is mapped to an IANA name (CLDR windowsZones table) | partial |
| Random bytes | xo_crypto.c | getrandom, getentropy | `BCryptGenRandom` (system preferred RNG) | done |
| TLS library (decision 0105) | third_party/mbedtls | pthreads | `xo/win/pthread.h` (SRW locks, condition variables) on the include path; Everest's Curve25519 built without `__int128` (kremlib's MSVC branch types it as `__m128i`); system roots from the `ROOT` certificate store (`CertEnumCertificatesInStore`) | done |
| Processes (Proc, decision 0107) | xo_proc.c | posix_spawn, waitpid, pipes, kill | `CreateProcessW` with an argument string quoted as Go's syscall.EscapeArg, anonymous pipes with overlapped reads or a reader thread, `WaitForSingleObject`, `TerminateProcess`, job objects for process groups | not yet (XO1504) |
| Memory mapping (allocator chunks) | xo_rt.c | malloc today | unchanged | n/a |
| Linking | internal/build | cc/clang + `-lm -lpthread` | clang `--target` default x86_64-pc-windows-msvc, `-fuse-ld=lld` (LTO bitcode), import libraries named by `#pragma comment(lib)` in xo_win.c (ws2_32, bcrypt, synchronization, shell32, crypt32) | done |

## Decision

1. **Target and toolchain: x86_64-pc-windows-msvc** (and
   aarch64-pc-windows-msvc next), clang with lld-link and the MSVC C
   runtime (ucrt), not MinGW. The binaries need only system DLLs, which
   is what a later Xo-native linker will produce too (decision 0195:
   platform libraries allowed).
2. **One runtime source, a compatibility layer.** `std/llrt/xo_win.c`,
   included by xo_rt.c under `_WIN32` instead of the POSIX headers,
   maps the POSIX subset the runtime calls onto Win32 and Winsock
   (function-like macros, so struct fields and parameters named `read`
   or `write` are untouched). Platform specific code that cannot be a
   shim (the fiber switch, fault unwinding, the poller's wait, console
   control) is `#ifdef _WIN32` in place.
3. **Faults unwind without libunwind.** fault_raise walks frames with the
   system unwinder and resumes at the nearest landing pad; the generated
   IR is unchanged (LLVM already emits the GCC call site table under
   `.seh_handler xo_personality, @unwind, @except`), so internal/llvm and
   selfhost/llvm need no target change for stage 1.
4. **Unsupported parts fail the build, never the run:** XO1504 (new,
   spec 10.6) names the std APIs whose runtime is not ported to the
   build's target. After stage 1 that is processes (Proc) only.
5. **Goldens:** `TestGoldenWant` (XO_GOLDEN_WANT=1) builds every LLVM
   golden with the LLVM backend alone and compares with
   `want_<goos>.txt` when the program's output is target dependent
   (`target.os`, permission bits), else `want.txt`, plus the leak check.

## Staged plan (ranked by likelihood of working)

1. **Stage 1 (this decision, done):** hello world, then threads, fibers,
   faults, timers, files, sockets, HTTP and TLS, standard input on
   windows/amd64: 139 of 143 goldens pass (bench/results/windows-2026-10-12.md).
2. **Stage 2: processes.** xo_proc.c on `CreateProcessW` (proc_cmd,
   net_only, unix_kill goldens: XO1504 today). Next experiment: port
   `xo_proc_run` first behind `#ifdef _WIN32` (quote argv as Go's
   `syscall.EscapeArg`, `STARTUPINFOEXW` with an explicit handle list for
   the pipes), run `XO_GOLDEN_WANT=1 go test -run 'TestGoldenWant/proc_cmd'
   ./internal/llvm` on the instance.
3. **Stage 3: windows/arm64.** The fiber switch for arm64 Windows (x19 to
   x28, fp, lr, d8 to d15, and the TEB's stack bounds at x18), the
   `CONTEXT` fields already used by fault_raise (Pc, Sp, X0, X1), and a
   host: EC2 has no Windows arm64 image (`describe-images` with
   platform windows and architecture arm64 returned 0 on 2026-10-12), so
   cross build from windows/amd64 with `--target=aarch64-pc-windows-msvc`
   and the VC ARM64 tools, and run on an Azure Cobalt VM or a Windows on
   Arm machine (the owner's choice). Next experiment: cross
   compile the runtime with `clang --target=aarch64-pc-windows-msvc -c
   std/llrt/xo_rt.c` on the instance (the VC ARM64 component is
   installed by scripts/remote-windows.sh).
4. **Stage 4: paths.** Absolute Windows paths (`C:\...`) in std's path
   handling: env_program fails on both backends because std/native and
   std/rt join the working directory with a drive path (the Go backend
   reports `Io(... The filename, directory name, or volume label syntax
   is incorrect.)`). A std/spec change: which paths are absolute, `\` as
   a separator on Windows.
5. **Stage 5: scale and fidelity.** IOCP (AFD poll) instead of WSAPoll;
   a stdin reader thread so reads are cancellation points; the local
   time zone from the registry; UTF-8 environment (`_wenviron`);
   a vectored handler for stack overflow; socket numbers kept apart from
   C runtime descriptors (today `close` tries `closesocket` first, a
   collision risk once a program has hundreds of files open).
6. **Stage 6: `xo test` and the remaining LLVM tests on Windows**
   (threads_test, signal_test, tls_diff, h2_diff), then CI for Windows
   on the EC2 script.

## Consequences

- `xo build --backend=llvm` and `xo run --backend=llvm` work on a
  windows/amd64 host with LLVM and the MSVC build tools installed; the
  binaries are PE executables linking only system DLLs (ucrt, kernel32,
  ws2_32, bcrypt, crypt32, shell32, the synchronization API set).
- Faults, deferred code, task isolation, and leak checks behave as on
  POSIX (every fault golden passes).
- Gotchas: Git for Windows checks goldens out with CRLF
  (gotchas/windows-git-crlf-goldens.md); a Windows socket shut down for
  receiving with unread input resets the connection
  (gotchas/windows-shutdown-resets-unread-input.md).
- No prebuilt runtime pack for Windows: LLVM builds compile the runtime
  locally (as on every LLVM host, decision 0157).
- The TLS library's key changes (xo/win/pthread.h is part of its tree),
  so every host rebuilds it once.

## Alternatives

- **MinGW (x86_64-w64-windows-gnu, llvm-mingw):** winpthreads, LLVM
  libunwind with SEH support, and more POSIX headers would have made the
  port smaller, and `_Unwind_ForcedUnwind` would work. Not chosen: the
  binaries depend on the MinGW runtime the toolchain ships, and the
  lead's task and decision 0195 point at the platform's own libraries.
  Still the fallback if the MSVC toolchain becomes a burden on users.
- **The Windows fiber API** (`CreateFiberEx`, `SwitchToFiber`) instead
  of the own switch: it maintains the TEB itself, but the scheduler's
  frame layout and the arm64 and x86-64 POSIX switches would diverge
  from it; the own switch with the three TEB words is what Boost.Context
  does.
- **IOCP now:** completion based I/O does not fit the readiness model
  the runtime's sockets, HTTP, and TLS callbacks are written for; WSAPoll
  keeps them unchanged. AFD polling through IOCP gives readiness at
  scale and is stage 5.
- **Unsupported features stubbed at run time** (a socket call failing
  with an error): a program would build and then fail in production;
  XO1504 at build time is the rule (as XO1201 for `--no-gc`).
