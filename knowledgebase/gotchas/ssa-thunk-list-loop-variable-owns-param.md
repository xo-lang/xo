# A List carried in a loop variable makes a thunk's parameter owned

Status: Observed (2026-10-20, decision 0291)

**Symptom.** A generated helper (an `xo.*` thunk built in compiler/ssa)
that only reads its struct parameter still costs reference counting at
every call: the caller increments the parameter's Lists before the call
and the helper decrements them on return. On the log Map prototype,
`xo.mlog.find` paid four helper calls per get (`_xo_h...` in the
disassembly), and an xo built with log Maps spent 22.9 s in insert_rc
against 8.3 s.

**Cause.** Borrow inference (compiler/ssa rc.xo infer_borrows) marks a
parameter owned when any path consumes one of its leaves. Writing a field
of the parameter into a local that a loop reads (`b.write_var(dv,
b.mf(t, 0, mp))`, then reassigning it from a parent level) passes the
List along a block edge, and an edge use consumes.

**Fix.** Keep only scalars in loop variables. Read the parameter's own
Lists directly where they are used, and read other Lists (a parent
level's) inside the loop body from an index the loop carries. Check
with `otool -tv` for helper calls in the thunk, or with insert_rc's
time in `--phase-times`.
