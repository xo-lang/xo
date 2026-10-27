# An unsafe range's marker label kept the function after it alive in the hollow pass

Status: Observed (2026-10-27: internal/arm64 TestDebugInfoSameCode std_error_display and error_text_http failed after
283716490, the opt-in pool cutoff, with "_xo_f_cabi__net_2enc__conn__free: no function with its code in the build
with debug information"). Fixed in compiler/arm64 hollow.xo (0024da60d).

Symptom: a build with `--debug-info=off` had live code (the network stack: cabi net conn_free, poll, netconn,
twrun, 25 functions and 3.6K lines of text) that the same program's build with debug information had hollowed to `brk`;
the same-code test compares the two. Nothing in the pool change touched them.

Cause: the hollow pass (decision 0371) took the unsafe-range markers `_xo_utext_X` and `_xo_utend_X` as roots. A
marker is a global label with no instruction before the next global label, so the scan puts it in the chunk of the
function that follows it; as a root it kept that function and everything that function reaches. Which function follows a
range's end depends on the layout of the runtime's functions, so any edit of the runtime could move the root onto a
function that reaches a lot (here `netpolicy.admits`, in the build without the DWARF readers).

Fix: a marker name keeps no chunk alive (hollow_live skips it as a root and as a reference). A hollowed function keeps
its labels, so the unsafe tables still resolve, to an empty range. Unit test: hollow_test.xo "an unsafe range's marker
keeps no function alive, only its own label".

Avoid: a label that names a place (a marker, a table entry) is not a reference to the code around it; a reachability
scan over text needs its own notion of what a label keeps alive.
