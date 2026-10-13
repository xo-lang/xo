# arm64 emitters: code written after the call site tables lands in their section

Status: Anticipated (2026-10-13, found reading the emitters, decision
0206 stage 2; not hit at run time).

Symptom (expected): a program whose functions have landing pads and
that uses `rt.sys_restorer` would install a signal restorer that is
not executable (it would sit in `__gcc_except_tab` /
`.gcc_except_table`), faulting when a handler returns.

Cause: the arm64 emitters (selfhost/arm64 gen.xo, internal/arm64
arm64.go) wrote the rt_sigreturn trampoline after `g.lsdas`, and each
LSDA switches the assembler to the call site table section without
switching back.

Fix: the trampolines (`xo_sigreturn`, and stage 2's `xo_clone`) are
written right after the helpers, while the text section is current.
Any code appended after the LSDAs must open the text section first.
