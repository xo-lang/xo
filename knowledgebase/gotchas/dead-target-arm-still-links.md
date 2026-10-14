# A C function only a dead `target` arm calls must still link

Status: Anticipated (2026-10-14, work item B16 part 2)

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
block on Windows). To delete the stubs too, fold `match target` in the
SSA builders and make the layer choice a constant the builders see.
