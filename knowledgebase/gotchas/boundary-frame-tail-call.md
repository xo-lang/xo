# A fault boundary frame's last call became a tail call

Status: Observed 2026-10-31 (LLVM backend, split builds at -O2).

## Symptom

`fault_task_exit` built with `XO_LLVM_SPLIT=6 XO_LLVM_LTO=none` at -O2 on
linux/arm64 exited 70 with two of its four defers: "main block: deferred"
and "main: deferred" never ran, while the fault's traceback still listed
`work` and `main`. It was deterministic (20 of 20, also on one CPU). Lang 2's
`fault_tasks` at split 3 failed the same way. It went away when any one of
the split's modules 1, 3 or 5 was built at -O0, and when a debug print was
added inside `unwind.raise`, so the failure looked like an unwinder
miscompile spread over three modules.

## Cause

`entry.main_on_fiber` stores the fault boundary (`ctx.C_BOUNDARY`) from
`rt.frame_addr()` and then calls main through `rt.call_c2`, its last
statement. The LLVM attribute set of a function that calls
`rt.frame_addr` (#9, and #15 under --race) had `noinline` and
`"frame-pointer"="all"` but not `"disable-tail-calls"`, so clang at -O2
emitted that call as a tail call. main then reused `main_on_fiber`'s frame
slot, and gdb's backtrace showed main called directly from
`sched.fiber_main`. main's CFA was above the boundary, so
`unwind.raise`'s stop test (`CFA > boundary`) ended the walk one frame
short of main's landing pad and `fault.exit` ran with no cleanups. The
traceback walk has no boundary test, so it still reached main. The
module split and the debug prints only moved code between modules, and
with it whether clang tail-called there; they were not the cause.

## Fix

Attribute sets #9 and #15 (compiler/llvm sched.xo frame_attrs, gen.xo)
carry `"disable-tail-calls"="true"`; func.xo gives them to every function
that calls `rt.frame_addr`. test/codegen/fnaddr checks the attribute.
A frame used as a boundary must stay below its callees on every backend.

## How to find the next one

When a fault ends a program with fewer cleanups than expected but the
traceback is complete, compare the fault's backtrace (gdb `bt` at
`fault.exit`) with the frames that set the boundary (`main_on_fiber`,
`tasks.run_entry`, `entry.run_main`). If the boundary frame is missing,
it was tail-called away. Debug prints in the unwinder change module
placement and can hide the failure; observe the unchanged binary with gdb.
