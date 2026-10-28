# A pass that copies a callee's body can copy an extern

Status: Observed (2026-10-28)

**Symptom.** A separately compiled build (XO_SEPCOMP, decision 0388 stage
3) faults in the compiler: `fault: index -1 out of range for List of
length 3 at ssa/rc.xo` in `Rc.walk`, for a program that passes a constant
function to a std function (`debug.router(os.clock, allow)`).

**Cause.** specialize_args (decision 0368) clones a callee for a call
site's constant argument. Under separate compilation std's functions are
externs (Module.externs): the program's module holds their facts but no
instructions, so the clone had a block with no terminator, which
reference counting indexed at -1.

**Fix.** spec_callee_at refuses a callee in Module.externs. Any pass that
reads or copies a callee's body (cloning, inlining, interprocedural
facts) checks Module.externs first; internal/arm64 TestSepcompLinks
builds such a program separately.
