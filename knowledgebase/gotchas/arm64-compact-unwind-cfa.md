# Darwin's compact unwind encoding ignores the CFA offset of a frame

Status: Observed (2026-10-09, decision 0085, golden `fault_audit`)

Symptom: an arm64 build crashed (SIGSEGV at pc 0, or SIGBUS) right
after `task.isolate` caught a fault raised in a closure: the landing pad
ran with the stack pointer 16 bytes too low, and main's epilogue loaded
its return address from the wrong slot.

Cause: a function whose frame record is at the bottom of a frame larger
than 16 bytes (`stp x29, x30, [sp, #-32]!; mov x29, sp`, no callee saved
registers) has the rule `.cfi_def_cfa w29, 32`. The integrated
assembler turns call frame information into a compact unwind encoding
when it can; for `def_cfa w29` followed by the x30 and x29 offsets it
picks frame mode, which means CFA = x29 + 16, without checking the
offset (LLVM's AArch64 generateCompactUnwindEncoding). The unwinder then
computed the caller's stack pointer from the wrong CFA. Functions with
callee saved registers in the backend's order fell back to DWARF and
were right, which hid it.

Fix: when the CFA is not x29 + 16, the backend writes x29's rule before
x30's (`internal/arm64/emit.go` cfiFrame); the encoder then declines frame
mode and the DWARF rules are used. Check `objdump --unwind-info` (encoding
0x03... is DWARF, 0x04... frame) when a frame layout changes.
