# 0213: Platform library calls from Xo (macOS libSystem, Windows API)

Status: Accepted (2026-10-13), stage 1 implemented: the platform call
mechanism (decision 0195 prerequisite 3 for macOS and Windows; decision
0206 is Linux's), std/native/sys_darwin.xo and sys_windows.xo,
std/native/llrt/darwin.xo and windows.xo, and std/native/llrt/sysio.xo
over the three OS layers; the clocks moved to Xo on every OS on
2026-10-14 (work item B01, item 6). Files: the declarations above; selfhost/ssa
call.xo (platform_call, target_os); selfhost/arm64 calls.xo
(platform_call), mir.xo, regalloc.xo, emit.xo (variadic arguments on
the stack); selfhost/llvm call.xo, gen.xo (goos); selfhost/wasm call.xo;
selfhost/xo main.xo; mirrored in internal/ssa build_native.go,
internal/arm64 isel.go, mir.go, regalloc.go, emit.go, and internal/llvm
fromssa.go. Tests: TestSysioParity and TestErrnoTables (internal/llvm
sysio_test.go, testdata/sysio_parity, testdata/errno_probe.c), the
golden sys_files_clock (now with want_windows.txt). Byte for byte
parity with the C path on darwin/arm64, linux/arm64, and windows/amd64
(TestSysioParity, 43 lines each). Evidence:
`bench/results/platform-calls-2026-10-13.md` (`bench/perf/sysio`).

## Context

Decision 0195 allows the runtime to call the platform's own libraries
where the kernel interface is not stable (macOS libSystem, the Windows
DLLs), as Go does, and its audit sketched how
(bench/results/zero-c-audit-2026-10-13.md, "Design sketches", (3)): a
declaration file per OS whose bodiless functions lower to direct calls
of the C symbols with the C ABI. Decision 0206 moved the files to Xo on
Linux over raw system calls (std/native/llrt/sysio.xo); on macOS and
Windows they stayed in the C runtime (std/llrt/xo_fs.c, xo_bytes.c, and
on Windows the POSIX shim xo_win.c over the Microsoft C runtime).

## Decision

1. **Declarations** (internal, decision 0074; no user visible change):
   std/native/sys_darwin.xo and sys_windows.xo, loaded like rt.xo
   (`use native/sys_<os>` from runtime modules only). Each bodiless
   function names its C symbol (`open`, `__error`, `CreateFileW`). The C
   ABI: an Int argument is one word (an int, size_t, HANDLE, or an
   address made with rt.bytes_addr), a Float a double, a Bytes argument
   the address of its data, kept alive across the call as the call's
   operand; an Int result is a word, an Int32 a C int (sign extended), a
   UInt32 a DWORD (zero extended), a Float a double, no result void.
   Parameters from the first one named `va_*` on are variadic arguments
   of the C function.
2. **SSA**: both builders lower a call to `Op.Call` named
   `rt.pc.<os>.<sym>`, with `/<n>` for a variadic function with n fixed
   parameters. A call for another OS than the build's target, or in a
   wasm module, is a fault (`rt.fault_raise`, then unreachable code):
   the passes keep every arm of a `match` on `target`, and the other
   OS's symbols do not exist at link time.
3. **Emitters**: selfhost/arm64 and internal/arm64 emit `bl _<sym>`
   with the arguments in the AAPCS64 registers and, on Apple arm64, the
   variadic ones on the stack, 8 bytes each (a call's `vfix`), then
   `sxtw` or a 32 bit move for an Int32 or UInt32 result. selfhost/llvm
   and internal/llvm declare the function with its C types (`i32` for
   Int32 and UInt32, `ptr` for Bytes, `...` after the fixed
   parameters; `dllimport` on Windows) and call it with the variadic
   function type where there is one; LLVM applies each target's
   convention (Apple's variadics, Win64). The wasm emitter faults (no
   platform library). selfhost/llvm's Gen now knows the target OS
   (goos, set by the driver).
4. **Layers** (linux.xo's interface: a result, or a negative errno):
   darwin.xo calls libSystem's open (variadic mode), read, write, close,
   stat, lstat, fstat (the 144 byte arm64 structure), clock_gettime,
   clock_gettime_nsec_np, getentropy, and reads errno through
   `__error()` right after a failed call; macOS's strerror table (ELAST
   107) and FsErr mapping. windows.xo calls kernel32's CreateFileW
   (with the arguments _wopen passes: shared read and write, not
   inherited, OPEN_EXISTING or CREATE_ALWAYS; FILE_READ_ATTRIBUTES with
   backup semantics for stat, the reparse point itself for lstat and
   for a link the system cannot follow), ReadFile, WriteFile,
   CloseHandle, GetLastError, GetFileInformationByHandle,
   GetFileAttributesW, GetFileType, GetSystemTimePreciseAsFileTime, and
   bcrypt's BCryptGenRandom; paths are converted to UTF-16 in Xo. Its
   errno is the C path's for the same failure: the Microsoft C
   runtime's _dosmaperr table for opens, reads, writes, and closes (as
   _wopen, _read, _write, _close set errno), xo_win.c's own map for
   stat, then xo_w_fix's refinement (EISDIR for an open of a directory
   refused, ENOTDIR when a parent is a file), and the Microsoft C
   runtime's strerror texts.
5. **Routing** (decision 0193's mechanism, `XO_LLRT_C=sysio` for the C
   path): sysio.xo is the portable policy (os.ReadFile's and
   os.WriteFile's loops, the status and message through rt.fs_set_err)
   over a per OS layer: Linux (decision 0206), macOS on arm64, Windows.
   os.fs.read, write, and stat (and read_dir's and walk's stats) are
   Xo on those three by default: on macOS the Xo path took 0.562 to
   0.996 times the C path's instructions retired (user and kernel) for
   read, read_big, and stat on both backends, and 0.978 to 1.013 for
   the kernel bound write over three runs (1.013, 1.006, 0.996 on arm64)
   (bench/results/platform-calls-2026-10-13.md), within decision 0180's
   bar and the noise; messages match the C path byte for byte (TestSysioParity,
   TestErrnoTables). darwin/amd64 stays on the C runtime: its plain
   `stat` symbols are the old structure
   (gotchas/darwin-amd64-stat-inode64.md).
6. **Clock.now stays on the C runtime** on every OS (superseded by the
   move below): from Xo on macOS
   it took 0.934 times the C path's instructions on LLVM but 1.042 on
   arm64, whose emitter does not fold the `match` on `target` that
   chooses the layer (gotchas/arm64-match-target-not-folded.md).
   darwin.realtime_ns (clock_gettime_nsec_np) and windows.clock_ns are
   in place for when it moves.
   **Moved (2026-10-14, work item B01)**: sysio.xo chooses the clock of
   the build once into word 0 of rt.sys_words (the vDSO address on
   Linux, a negative code for libSystem or kernel32), so Clock.now tests
   no `target`; macOS calls clock_gettime_nsec_np inline and Windows
   windows.clock_ns (GetSystemTimePreciseAsFileTime). The monotonic
   clock is sysio.mono_ns on every OS (CLOCK_MONOTONIC through the vDSO
   or libSystem, QueryPerformanceCounter on Windows), read by pool.xo
   and, through cabi.xo's mono_ns and real_ns, by the C runtime
   (mono_ns, xo_clock_now's real time, xo_tz_now); xo_win.c's
   clock_gettime shim is deleted. The virtual clock of the test world
   stays the C runtime's (rt.sys_clock_buf answers 0 while it runs).
   Clock.now on macOS: see bench/results/clocks-2026-10-14.md.

### Next steps

- Fold a branch or switch on a constant in selfhost/arm64 (Clock.now
  no longer waits for it: it moved with a word chosen once, item 6;
  pool.xo's other `match layer()` calls and sysio's file layers still
  pay the compares).
- rand.bytes and rand.int from Xo: darwin.random (getentropy) and
  windows.random (BCryptGenRandom) exist; the C runtime's seeded test
  world (tw.on) needs a test intrinsic as rt.tw_fake is for files.
- darwin/amd64: a way to name `stat$INODE64` (a symbol name in the
  declaration), then the Intel Mac run (scripts/intel-mac-test.sh).
- The rest of the Fs operations (read_dir's names, rename, remove,
  mkdir, chmod, symlink, handles), then threads, the poller (kqueue,
  `__ulock_wait`; IOCP and AFD on Windows through ntdll), sockets
  (ws2_32), and C ABI entry points (thread starts, signal and console
  handlers), each against its C path; xo_win.c shrinks as the Windows
  layer grows. Functions of ntdll need its import library on the
  link line (kernel32, bcrypt, and ws2_32 come with xo_win.c's
  `#pragma comment(lib)`), once the C runtime no longer names them.
- The Windows _dosmaperr table is checked only through the errors
  TestSysioParity provokes (the C runtime does not export its map); a
  probe that opens paths failing with each Win32 error would cover the
  rest.

## Consequences

- macOS and Windows builds call libSystem and kernel32 from Xo for the
  file paths; the C runtime keeps its own copies for `XO_LLRT_C=sysio`
  and for the operations not moved.
- Every target compiles every OS layer's Xo (the `match` arms are not
  folded), but only the target's platform calls reach the object file.
- A Go-side mirror keeps the Go driver (`XO_SELF_DRIVER=0`) and the
  selfhost differential tests building the same programs (decision
  0178's freeze allows it to keep the Go side passing its tests).

## Alternatives

- Raw system calls on macOS (`svc #0x80`, x16): unsupported by Apple,
  the numbers and structures change between releases; Go moved off
  them in Go 1.11.
- Through the C runtime (C wrappers per call): keeps C in the path,
  against decision 0195.
- A symbol table of variadic functions in the emitters instead of
  `va_*` parameters: hides the ABI fact from the declaration.
- Native Windows NT calls (NtCreateFile through ntdll): undocumented
  semantics and errors, and the C path's messages come from the Win32
  layer's error codes.
