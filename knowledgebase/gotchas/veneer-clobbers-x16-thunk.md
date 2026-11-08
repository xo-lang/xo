# A branch veneer clobbers x16: `bl THUNK_x16` past 128 MB

Status: Observed (2026-11-07, Azure 1's GCP compute/v0.alpha and
aiplatform/v1 tests on linux/arm64, diagnosed by Xo Lang 4)

Symptom: a binary with more than 128 MB of text links (since f895f73a4's
ELF branch islands) but crashes: `SIGSEGV (code 2)` at an address a few
hundred bytes past the pc, in `drop Str` under the caller; compute's
first 215 tests (the ones whose code was farthest from the thunks), all
of aiplatform/v1's.

Cause: a retain or release of a Str is `bl THUNK_xN` with the object in
xN (compiler/arm64 rc.xo inc_thunk, drop_thunk; INC_THUNK_REGS includes
16 and 17), and the x16 entry has no `mov`, the object being in x16
already. A far `bl` goes through a linker veneer, `adrp x16; add x16;
br x16`: AAPCS64 lets a veneer clobber IP0 and IP1, and both of Xo's
linkers (Mach-O islands since 2026-10-22, ELF since 2026-11-07) use x16.
The thunk then counted the veneer's target, an address in the code: a
write into text. A spilled object is reloaded into x16 for these
instructions, so the x16 entry is common (http_client: 1,846 of 4,397
thunk calls).

Fix: emit_plain turns a thunk call whose object is in x16 into `mov x17,
x16; bl THUNK_x17` (the call clobbers x17 already, and the x17 entries
survive a veneer: the drop thunk moves x17 to x16 after it, the retain
uses x16 only as scratch). Test: internal/arm64 TestGoldenRunIslands
runs goldens with XO_BRANCH_REACH small (veneers in a small program),
with and without XO_ARM64_REGS=4. Rule: nothing may reach a callee in
x16 or x17 through a `bl` or `b` that a linker can route through a
veneer; ordinary calls clobber x0 to x18, so only hand written call
conventions (thunks, helpers called with a register argument) can.

Second trap (the first fix did not fire): emit_plain's `use_reg` list is
padded to at least 10 entries (`imax(inst.us.len(), 10)`), so a test of
`use_reg.len() == 1` is never true; the fix compiled, xo built, and the
asm still had every `bl THUNK_x16` (http_server: 1,961 of them, no `mov
x17, x16`). Count an instruction's operands with `inst.us.len()`, and
check a codegen fix by grepping the asm it was meant to change before
running tests.
