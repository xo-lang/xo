# windows/amd64: the runtime calls generated C ABI functions with Win64

Status: Observed (2026-10-19, Wine under Box64), fixed in compiler/arm64
x86win.xo (c_entry_sym, w64_exports).

Symptom: on windows/amd64 with the amd64 backend, an empty `s.spawn`
crashed in cabi.task_done writing to 0x70; a task sending on a buffered
channel never ran (XO-F001 deadlock).

Cause: the runtime calls a task's entry function (T_RUN), a list's
element drop, and a constant's builder through rt.call_c2 and
rt.call_c4: C ABI calls, so Win64 on Windows. The SSA builder marks
those generated functions Func.entry; the native backend compiled them
System V like every Xo function (decision 0254 6c), so they read their
arguments from rdi and rsi while the call had put them in rcx and rdx.

Fix: each Func.entry function gets a Win64 to System V shim
(xo_w64e_<symbol>), and FuncAddr of one takes the shim's address.
Exports are Win64 shims the same way; generated code's direct calls go
to the functions themselves.
