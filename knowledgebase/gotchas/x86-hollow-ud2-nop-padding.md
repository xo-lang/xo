# An x86 disassembly's ud2 is not a hollow marker, and a hollowed body has nops after it

Status: Observed (2026-10-25, the amd64 hollow port).

Symptom: while checking a darwin/amd64 build by hand, a script that
printed `label -> instruction` for every disassembled instruction showed
`_xo_f_main -> ud2` and looked like the pass had hollowed main. The
compiler's own assembly (xo build's --asm output) showed main was not
hollowed at all.

Cause, two of them:

- The x86 backend emits `ud2` itself as the dead end of its fault paths
  (`callq _xo_x_fault_raise_index` then `ud2`), and an arm64 `brk`
  carries the HOLLOW_BRK immediate that names the pass; x86 has no
  immediate to carry a marker. A ud2 in a body therefore says nothing on
  its own: a hollowed body is the function's only instruction, one ud2.
- A hollowed body is 2 bytes (`ud2`) and the function's `.p2align`
  padding follows it, so otool's disassembly of that body is `ud2` and
  then `nopl`/`nopw` (arm64's `brk` is 4 bytes and needed no padding). A
  test that requires a one instruction body finds no hollowed x86
  function.

Fix: compare the raw assembly text, or filter `nop` instructions out of
a disassembled body before counting it (internal/arm64/dce_test.go's
TestHollowUnreachedAmd64 does), and read a __text change plus the
compiler's own counts rather than a single instruction.
