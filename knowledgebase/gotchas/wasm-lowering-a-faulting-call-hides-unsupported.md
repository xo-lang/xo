# Lowering a call that used to fault turns "unsupported" goldens into traps

Status: Observed (2026-11-03, decision 0447 R2d).

## Symptom

After rt.call_c2 and rt.call_c4 were lowered as `call_indirect` on wasm (the
TLS ops table), TestSelfhostWasm failed three wasip1 goldens that passed
before: dns_no_tasks and term_stdio with `wasm trap: null function or
function signature mismatch`, and stdin_long_line with an I/O error.

## Cause

Those goldens reach rt.call_c2 at run time (obj.xo's heap hooks, alloc's
heap_big): the target is a word loaded from memory, a native hook or 0, not
a table index. Before, the call faulted with "the wasm backend does not
support the call `rt.call_c2`", and internal/selfhost/wasm_test.go's
wasmCCallRE counts a run that prints it as unsupported, so the goldens were
never really passing; they were counted as unsupported by their message.
The call_indirect removed the message and called whatever sat in the table
at that index.

## Fix

Keep the fault for every target that is not one the new lowering is for:
call_c on wasm calls through the table only when `$cabi_ok` says the index
is a C ABI export's slot (func.xo records each cabi FuncAddr's table entry;
gen.xo writes the predicate with the table), and otherwise faults with the
old message. When a lowering replaces a faulting call, check who reaches it
with other operands, and run TestSelfhostWasm (wasip1) as well as the
command goldens.
