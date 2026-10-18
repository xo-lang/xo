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

## Windows only runtime costs, read from the code (2026-10-18)

Under Wine neither wall time nor instruction counts stand for Windows
(Wine implements IOCP, the waits, and the loader itself, and valgrind
does not run Wine under Box64), so this pass reads the windows/amd64
LLVM build of bench/perf/conc spawn (llvm-objdump) against the Linux
one, per operation:

- The task switch (xo_stack_switch, compiler/llvm sched.xo): 57
  instructions on Windows against 21 on Linux. The 36 more: xmm6 to
  xmm15 saved and restored (20; Win64 makes them callee saved, System V
  none), the TEB's StackBase, StackLimit, and DeallocationStack switched
  with the stack (12; needed: the stack overflow handler and SEH read
  them), rdi and rsi (4).
- Thread locals (xo_tlocal_get, _set, _addr; out of line on every OS, a
  fiber may resume on another thread): 4 instructions plus the call on
  Windows (`_tls_index`, `gs:0x58`, the module's slot, the word) where
  Linux is one load from `%fs`.

Tried and dropped: the switch alone with the System V convention
(x86_64_sysvcc; its body 33 instructions). Its three callers
(pool.park_wait, pool.run_item, fiber_main), Win64 functions, then save
xmm6 to xmm15 themselves in their prologues (20 instructions; none
before), and park_wait and run_item switch once a call, so the cost only
moves. It goes away only where Xo's own calls use System V throughout,
with Win64 at the OS boundary (exports, callbacks the OS makes, platform
calls): a convention decision for the LLVM backend's Windows builds (and
decision 0254 stage 6b proposes the same for the native one). A thread
local through a TlsAlloc slot in the TEB (`gs:0x1480 + 8 * slot`, as Go
does) would save the `_tls_index` load and one indirection per access.
Neither can be measured here: that needs a Windows machine
(scripts/remote-windows.sh, the lead's).
