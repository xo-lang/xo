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

Fix (5afd8f98c): no stub saves on Windows, as a Windows leaf's jumping
stub already had none; the faulting frame's caller saved variables
printed `<unavailable>` there. Then (2026-10-30): a Windows function
whose stubs save is emitted again (x86_emit, as for borrowed registers)
with X86_STUB_SAVES frame words above its by reference memory (stub_at),
and its stubs store there with rsp unchanged, so the prologue's unwind
data stays right and the debug info (DWARF, the PDB's locals) places
those variables again, and XO_TRACEBACK=locals prints them (on Windows
since 2026-10-30: unwind.xo win_walk_regs, gotcha windows-locals-probe.md). TestWindowsAmd64Identity turns the saves
off on both platforms (XO_PASSES=-x86_stub_saves). Any other rsp change
outside the prologue on Windows needs unwind data of its own (or must
not hold across a call): check the Windows goldens on the offload host
(`OFFLOAD_ENV=offload-win.env amd64-run.sh`), not Wine, for unwinding.
