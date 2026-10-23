# Code lowered before any statement has line 0: tracebacks name the wrong frame

Status: Observed 2026-10-23 (field report on 0.26.0), fixed

Symptom: a failed `requires` printed a traceback whose top frame was
`prelude.llrt_concat_int (prelude.xo:2703) [inlined]`, a string helper,
followed by `slug (slug.xo:0)`.

Cause: SSA values take the current statement's line (Builder.s.line,
set by stmt; decision 0239's line tables). A function's `requires` and
`ensures` checks are lowered at its entry and exit, outside any
statement, so their values, the failure's detail text (whose string
concatenation inlines llrt_concat_int) and the raise had line 0. With no
row of its own, the raise's address took the last row before it, the
inlined helper's, and the function's own frame got line 0.

Fix: compiler/ssa contract.xo check_requires and contract_check set the
line to the clause's condition while they lower it. Any other code the
builder makes outside a statement (wrappers, implicit drops at entry)
needs the same: set_line from the construct it lowers. Test
internal/arm64 TestTracebackContract (arm64 and LLVM).
