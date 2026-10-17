# A C function only a dead `target` arm calls must still link

Status: Observed (2026-10-14, work item B16 part 2; fixed in the SSA
builders 2026-10-15)

Symptom (expected): deleting a C runtime function on one OS because the
Xo code there no longer calls it (its call sits in a `match target` arm,
or behind a layer number such as `fsops.layer() == 0`, that this OS
never takes) links in a release build and fails in a development one:
an undefined symbol `xo_fs_rename` (and so on) on Windows.

Cause: every arm of a `match` on `target` lowers (spec core 12.2 says
only the chosen arm is generated, but no SSA pass folds it:
gotchas/arm64-match-target-not-folded.md,
gotchas/wasm-target-os-is-the-host.md). clang at -O2 drops the dead
call; the Go tests and scripts/bootstrap-xo.sh build with
XO_LLVM_OPT=O0 (decision 0220), which keeps it, and a layer number
returned by another function is not a constant to the emitters at all.
Platform calls (sys_<os>.xo) do not have the problem: the SSA builders
make a call for another OS a fault.

Fix: on the OS that no longer uses them, compile the C bodies out and
keep one line stubs that fault if ever run (std/llrt/xo_fs.c's FS_DEAD
block, on every native target since darwin/amd64 left layer 0, and
xo_bytes.c's xo_fs_read). To delete the stubs too, fold `match target` in the
SSA builders and make the layer choice a constant the builders see.

Fixed for the wasm layer 0 calls (2026-10-15): the SSA builders lower
a call of rt.fs_* (but fs_root, fs_mode, fs_new, fs_canceled) to a fault
on the native targets, and rt.stdin_* off Windows, as they do a platform
call of another OS (compiler/ssa call.xo wasm_only_rt). The C stubs
(xo_fs.c FS_DEAD, xo_sys.c, xo_stdin.c's non-Windows half) stayed only
while the seed's code still named them
(runtime-c-the-seed-still-calls.md); they were deleted with seed
0.12.0-26641183 (2026-10-15).

Fixed in the SSA builders 2026-10-16 (decision 0246): `if` and `match`
on target.os and target.arch (and on the OS layer number, sysio.layer
and netconn.os, against a literal) lower only the arm taken, and a block
stops lowering after a return, so another OS's code after an early
return is not built either. A choice made at run time (a word set at
start) still keeps every OS's code; write it as a `target` test.
