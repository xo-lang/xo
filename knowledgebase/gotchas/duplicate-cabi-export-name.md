# Two cabi modules exporting the same name fail only at assembly

Status: Observed (2026-10-15, decision 0233 step 2).

Symptom: every program fails to build with `error: symbol
'_xo_x_poll_wait' is already defined` from the assembler (or the
linker's duplicate symbol on LLVM), after a new export was added to
cabi.xo. `xo check` and the SSA builder say nothing.

Cause: every top level function of a cabi*.xo module becomes the C ABI
symbol `xo_x_<name>` (decision 0207), and the modules share that one
namespace. cabi.xo's new `poll_wait` (a park wait for poll.xo) collided
with cabi_poll.xo's `poll_wait` (epoll_wait). No pass compares the
export names of different modules, so the clash shows up only when the
backend writes both symbols.

Fix: give the export a name no cabi module has (`poll_park_wait`).
Before adding one, `grep -n "^fn <name>" std/native/llrt/cabi*.xo`. A
check in selfhost/ssa build.xo declare_exports_of, reporting a name
two export modules define, would turn this into a compile error.
