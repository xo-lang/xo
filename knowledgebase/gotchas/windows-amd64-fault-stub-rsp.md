# windows/amd64: a fault stub that moves rsp breaks the unwind to task.isolate

Status: Observed (2026-10-30, golden fault_audit on native windows/amd64
through the Windows offload host)

Symptom: a fault in a function with a frame exits 70 with `fault: ...`
and a one frame traceback (`main.fn1`) instead of reaching the enclosing
`task.isolate`; fault_audit printed nothing on stdout. Faults whose stub
saved no register (List index, `fault(...)`) were caught, so 29 other
isolate goldens passed.

Cause: x86emit.xo's fault stubs store the caller saved registers that
hold variables below rsp first (`subq $N, %rsp` and stores, stub_saves,
for the variables a fault report and the debugger show). The Win64
unwind data describes only the prologue's rsp, so RtlVirtualUnwind from
the stub's call read the return address N bytes off, found no function
there, and win_raise and win_walk stopped (unwind.xo).

Fix: no stub saves on Windows (x86emit.xo, `g.os != "windows"`), as a
Windows leaf's jumping stub already had none. TestWindowsAmd64Identity's
foldStubs drops the Linux saves before comparing. Any other rsp change
outside the prologue on Windows needs unwind data of its own (or must
not hold across a call): check the Windows goldens on the offload host
(`OFFLOAD_ENV=offload-win.env amd64-run.sh`), not Wine, for unwinding.
