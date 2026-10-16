# x86_64 unwinding: a landing pad expects DW_CFA_GNU_args_size added to the stack pointer

Status: Observed (2026-10-15, xo-fa, the goldens on linux/amd64 in
Docker, emulated).

## Symptom

On linux/amd64 (LLVM backend), a fault that unwinds through some frames
crashes instead of being reported or caught: cli_decl_fault exits 139
where it should report the fault and exit 70, http_conn_fault exits 139,
and http_fault's handler fault is not recovered (exit 70). Plain faults,
faults with a pending defer, and fault_unwind pass. arm64 never shows it.

## Cause

For a call with more than six integer arguments x86_64 LLVM may push the
stack arguments and record their size with DW_CFA_GNU_args_size. A
landing pad of that call expects them popped. The stack pointer the
unwinder computes is the one at the call, so libgcc adds args_size to it
when it installs a landing pad. std/native/llrt unwind.xo read the
opcode and dropped its value, so the cleanup ran with rsp off by the
pushed bytes and its `ret` went to an address in data. std/cli's
`cli.command` (many named arguments) made such calls.

## Fix

unwind.xo records the frame's args_size (H_ARGS, cleared for each frame)
and personality adds it to the stack pointer word before
_URC_INSTALL_CONTEXT. arm64 code never has the opcode.
