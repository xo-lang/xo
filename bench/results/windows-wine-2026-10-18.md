# Windows goldens under Wine (windows/amd64, LLVM; 2026-10-18)

The owner put Windows back into the regular work (2026-10-18): not a
gate (arm64 only), but a run per batch of runtime or backend changes,
recorded here. This is the first, on main a44ea332 (the day's suspects:
rt.mem_copy_block, the unwinder's frame cache, task and alloc changes,
fs create_new, IO chunking).

Method: scripts/wine-goldens.sh (new): every golden of
internal/llvm/testdata/golden and the Windows only ones of
internal/llvm/testdata/windows cross built on the Mac with
`--backend=llvm --target=windows/amd64` (the winxc toolchain, 3 s each),
then run under Wine 10.0 with Box64 0.3.4 in a linux/arm64 container
(scripts/docker-wine.sh box64), each program in a fresh wineserver,
Box64's own lines dropped from its standard output, 60 s limit,
want_windows.txt where a golden has one.

## Result

186 of 192 pass, run after run; no failure is an Xo bug.

- Wine gaps (7): fs_files, fs_ops_edges, fs_create_new (symbolic
  links), fs_paths_os (the \\localhost\C$ share), proc_cmd and net_only's
  process line (no sh, echo, tr), unix_kill (no AF_UNIX: "socket: address
  family not supported").
- Box64 flakiness: in a full run about six programs crash inside Wine's
  own code ("Unhandled illegal instruction at 00006FFF...", Wine's
  built in DLLs) and pass when run again: in the definitive run
  borrow_proj, conc_closure_env, newtypes, select_arm_order,
  std_error_display and std_path (3 runs each afterwards: 17 of 18 pass,
  the failure again inside Wine).
- The first two runs failed many more (94 of the first 130, then every
  program timed out; then 36 with extra output lines): the harness, not
  the programs (gotchas/wine-box64-harness.md). env_case failed only in
  the prefix those runs had damaged (no PATH at all) and passes in a
  fresh one.

Next per the owner's list: a run per batch of runtime or backend changes
(this file, a dated section each), and profiles of the Windows only
runtime paths (IOCP, thread local access, the scheduler's waits) by
instruction counts.
