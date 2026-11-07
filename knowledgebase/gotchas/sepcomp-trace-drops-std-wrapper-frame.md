# A separately compiled program's traceback has no prelude.llrt_* frame

Status: Observed, explained (2026-11-06, linux/arm64 host 2 and the Mac, xo-lang-4); no longer seen for atomic_ops and conc_send_closed since 2026-11-07 (Update below)

## Symptom

The same golden built whole program and separately compiled (decision
0388, std's variants prebuilt) prints different tracebacks after a
fault. atomic_ops, whole:

    atomic.add (atomic.xo:42) [inlined]
    prelude.llrt_atomic_add (prelude.xo:4884) [inlined]
    bump (main.xo:8) [inlined]
    main (main.xo:49)

separately compiled:

    atomic.add (atomic.xo:42)
    bump (main.xo:8) [inlined]
    main (main.xo:49)

conc_send_closed loses prelude.llrt_chan_send_w the same way;
sized_overflow, bytesbuilder_put_fault and rune_fault show the same
frames without "[inlined]". The exit status, the fault's line and all
output are the same: of 596 goldens compared whole against separate
(2026-11-06), these 5 differ, only in their trace frames. want.txt keeps
only a fault's first line, so no golden suite sees it.

## Cause

Decision 0224's tail calls. A method call such as `a.add(n)` lowers to
a prelude.llrt_* wrapper whose body is one call (`atomic.add(a, n,
at)`). Separately compiled, the wrapper is std's own function in std's
store and the program calls it: the native inliner never inlines an
extern std function (compiler/arm64 inline.xo, m.is_extern), and the
wrapper is one instruction, `b _xo_f_atomic_2eadd`, so its frame is
gone by the time the callee faults ("a tail call drops the caller's
frame from a backtrace taken inside the callee", 0224 Consequences).
Whole program, the wrapper and the method inline into the caller and
its DWARF names each as an inlined frame. The program object's DWARF is
complete either way.

## Fix

None needed: the frames a reader acts on (the std method, the
program's functions, main) are all there; only the compiler's lowering
glue is missing. internal/arm64 TestTracebackSeparate pins both traces
for atomic_ops and conc_send_closed. Keeping the wrappers' frames would
mean no tail calls out of prelude.llrt_* (a frame on each such call in
std, an amendment of 0224 for the owner); a compare of whole against
separate traces should drop "[inlined]" and prelude.llrt_* lines first.

## Update (2026-11-07)

Tier 58's sep step (scripts/lead tier.sh, 9603cd3f3) and the Mac alone at
5dc30fa68: std's variant now inlines the method into its wrapper, so
the separately compiled traces have the wrapper back, as a frame of its
own, with the method "[inlined]" in it:

    atomic.add (atomic.xo:42) [inlined]
    prelude.llrt_atomic_add (prelude.xo:4958)
    bump (main.xo:8) [inlined]
    main (main.xo:49)

and the same for conc_send_closed (chan.send_w [inlined],
prelude.llrt_chan_send_w, main). Every frame of the whole program's
trace is there; only the wrapper's "[inlined]" differs. The wrapper is
no longer a single tail call once its callee is inlined, so 0224's
dropped frame no longer applies to these two.
TestTracebackSeparate pins the new form. Which inliner change moved
these callees into their wrappers in std's variant was not bisected; a
later change that turns them back into tail calls fails the test the
other way, and either form is a correct trace.
