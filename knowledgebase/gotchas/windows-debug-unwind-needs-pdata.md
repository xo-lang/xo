# Windows: a debugger's unwind needs .pdata, and a step into stops before the parameters

Status: Observed (2026-11-05, Xo Lang 2, decision 0253 stage 6)

## Symptom

`xo debug` on windows/amd64 (cmd/xo tests on the Windows offload host):
TestDebugTasks and TestDebugAttach read a parked task's `v` as 1, not 0;
TestDebugStep's `s` into `fact` stopped at `fact at main.xo:2` with
`p n` answering `<optimized out>`; TestDebugDAP's breakpoints were
refused (`no file "C:\...\main.xo"`, the line table has `./main.xo`)
and a std frame's source became `CWD/C:/...`.

## Cause

1. A PE has no .eh_frame. Without a table the unwinder followed the rbp
   chain, which gives the return address but not the callee saved
   registers a frame pushed (rbx, rsi, rdi, r12 to r15), so a caller's
   variable kept in one read the callee's value.
2. A Windows function's line table starts its first line at the entry,
   before the prologue moves the parameters to where their locations
   say; a step that stops at the first statement row of the new frame is
   too early (other targets' locations happened to cover the entry).
3. The editor's path has backslashes and a drive, and DAP took only a
   leading `/` as absolute.

## Fix

compiler/debug winunwind.xo unwinds by .pdata's RUNTIME_FUNCTIONs and
UNWIND_INFO (and an epilog's instructions at the first frame); step into
a function runs on to its after-prologue address (where `break FUNCTION`
stops); breakpoint files match with either separator and a drive's path
in any case, and dap.xo's is_abs_path knows drive and UNC paths. Tests:
the seven TestDebug* on windows/amd64, compiler/debug arch_test.xo, and
TestDebugWindowsUnwindData on any host (`xo dev debug --unwind-check
EXE` checks a Windows executable's .pdata).
