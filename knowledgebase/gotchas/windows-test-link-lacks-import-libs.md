# internal/xotool CompileC links a Windows program without the import libraries xo's own link adds

Status: Observed (CI windows suite 40926030751, 2026-10-29)


## Symptom

On xo-windows, TestRuntimeThreads/Tasks/EventLoop, TestH2Diff and every TestGoldenTasks case
fail at link: `lld-link: error: undefined symbol: __declspec(dllimport) WakeByAddressSingle`
(referenced by chan.recv_w and the runtime probes), after "no ThreadSanitizer for this target,
testing without it".

## Cause

 since 38c7c8c28 (TestGoldenTasks builds each golden from its LLVM IR alone) and 4b3c43301
(--race links xo's ThreadSanitizer runtime), those tests link the IR through xotool.CompileC
(internal/xotool/cc.go compileC), whose Windows line is `-fuse-ld=lld -Wl,/Brepro` plus the
caller's -l flags; xo's own link (compiler/xo link.xo sys_link_args) adds kernel32, ws2_32, bcrypt,
synchronization, ntdll, shell32 and crypt32. The futex (pool.xo, sys_windows.xo's dllimport
WaitOnAddress/WakeByAddressSingle, sched stage 2 d65d5c448) was already in the last passing suite
(38630848188, 2026-10-14); only the link path changed. No windows suite ran between 2026-10-14 and
2026-10-29, so it stayed hidden.

## Fix

 compileC appends the Windows import libraries (share link.xo's list, one source), or the
race/golden harness passes sys_link_args' -l flags. A Mac link needs none, so a Mac gate cannot see it.

Fixed: internal/xotool WindowsLinkLibs (one list), compileC appends it on
windows, TestWindowsLinkLibs holds it equal to link.xo's sys_link_args.
