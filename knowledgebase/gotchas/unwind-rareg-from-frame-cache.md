# Faults isolated past a compact frame went uncaught on darwin/amd64

Status: Observed (2026-10-18, the owner's Intel Mac, LLVM backend, main
a90afcab; Xo Lang 3)

Symptom: on darwin/amd64 13 tests faulted with exit 70 and `fault: ...`
where task.isolate (or a server's handler isolation) should have caught
the fault: byref_held, fault_audit, fault_defer_block, fault_defer_chain,
fault_task_exit, fault_tasks, fault_unwind, http_conn_fault, http_fault,
iface_eq, native_gaps, task_native, TestRealtimeNative. Faults raised in
a function with a `defer` were caught (fault_unwind's first four), the
first one raised in a function without one was not. linux/amd64 and
darwin/arm64 passed.

Cause: std/native/llrt/unwind.xo's frame cache (lookup_frame,
fc_restore) saved and restored H_RAREG, the CIE's return address
register, with every frame, compact frames included. A walk's first
compact frame was cached before any CIE had been read, with H_RAREG 0.
A later walk restored it, then met a DWARF frame whose CIE was still the
cached one (H_CIE_ADDR), so read_fde skipped read_cie and left H_RAREG
0: step took the return address from register 0 (rax on x86_64), got 0,
and the walk ended as if at the end of the stack. Only darwin/amd64
mixes compact (__unwind_info) and DWARF (__eh_frame) frames in one walk:
the LLVM backend's darwin/arm64 frames all took the DWARF path in a
trace, and Linux has no compact unwind.

Fix: read_cie also keeps the return address register in H_CIE_RAREG,
and read_fde sets H_RAREG from it every time, as it sets H_PERS from
H_CIE_PERS. Found with a traced build (a line per frame: pc, encoding,
sp, personality) run on the Intel Mac; checked there with the 12
goldens' binaries cross-built on the Apple silicon Mac (want.txt's exit
and stdout) and testdata/realtime/alloc.
