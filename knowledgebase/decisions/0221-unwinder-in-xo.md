# 0221: The fault unwinder in Xo (no libgcc), XO_NOLIBC Linux first

Status: Accepted (2026-10-14, the lead's B05 assignment from the zero-C
audit, bench/results/zero-c-audit-refresh-2026-10-13.md item B05 and row
11 of its ordered list). Stage 1 implemented for XO_NOLIBC linux/arm64
(both native backends); stages 2 and 3 not started.

Evidence (stage 1, commit c38a15f8, linux/arm64 warm EC2 r8g, xo built
from the checkout): with XO_NOLIBC=1 the 12 fault goldens that failed
pass, 12 of 12 on the arm64 backend and 12 of 12 on LLVM (`xo dev
goldens --force --only=...`); every golden of internal/llvm/testdata/golden
with XO_NOLIBC=1: arm64 143 of 154 pass, LLVM 144 of 154, every failure a
build that does not link with no C library, as before (7 std/http TLS
refusals; proc_cmd, term_stdio, unix_kill; arith on arm64 only), none
failing at run time (before: arm64 129 pass and 12 fail of 153, LLVM 130
and 12; bench/results/linux-syscalls-2026-10-13.md, "Golden matrix").

## Context

A fault is raised as a forced unwind (decision 0085): xo_rt.c's
fault_raise calls `_Unwind_ForcedUnwind` (libgcc on Linux, libunwind on
macOS) with a stop function (fault_stop: frames past the context's
boundary are the runtime's) and the personality routine xo_personality,
which reads the GCC call site table both backends emit and enters the
landing pad of the call site. Windows walks the frames itself over
RtlVirtualUnwind (decision 0197).

A program linked with no C library (XO_NOLIBC=1, decision 0206) has no
libgcc: xo_nolibc.c defined `_Unwind_ForcedUnwind` and seven other
`_Unwind_*` functions as stubs that find no frames, so a fault ended the
program at once, without running deferred code, and a fault in a task
could not be caught. 12 goldens failed that way on every backend
(fault_unwind, fault_tasks, fault_defer_chain, fault_exit,
fault_task_exit, fault_audit, byref_held, iface_eq, native_gaps,
nested_update, realtime_section, task_native;
bench/results/linux-syscalls-2026-10-13.md, "Golden matrix").

Decision 0195 (no compiled C runtime) needs the unwinder out of C; the
audit lists it as three items: a CFI interpreter, a register restore and
resume primitive, and the personality routine with its LSDA reader.

## Decision

1. **The unwinder is Xo**: std/native/llrt/unwind.xo. `unwind.raise(exc,
   boundary)` saves the caller's registers (rt.unwind_save), finds each
   frame's FDE by binary search in the sorted table of the program's
   `.eh_frame_hdr` (its PT_GNU_EH_FRAME segment, found through
   AT_PHDR), reads the CIE (augmentations z, L, R, P, S, B, G), runs the
   DWARF call frame instructions up to the frame's return address
   (libgcc's execute_cfa_program: every DW_CFA_* but the expression
   forms, which end the walk; remember and restore state up to 8 deep;
   arm64's negate_ra_state, the return address's pointer authentication
   bits cleared), and computes the caller's registers (uw_update_context).
   At each frame it does what fault_stop and xo_personality did: past
   the boundary (the frame's stack pointer at its call above it) it
   returns; a frame with a call site table goes to `unwind.personality`,
   and at a landing pad the frame's registers get the exception object in
   x0 and 0 in x1 and the pad as the pc, and rt.unwind_resume jumps
   there. It returns 0 when nothing catches the fault (the end of the
   stack, no FDE, or unwind data it does not read); fault_raise then
   reports the fault as before (fault_exit).
2. **Two intrinsics** (internal, decision 0074): `rt.unwind_save(regs)`
   stores x19 to x30, sp, and d8 to d15 at regs by DWARF register number
   (word n is register n; the return address is x30's word), and
   `rt.unwind_resume(regs)` loads them, x0 and x1, and sp, and branches
   to word 96. Both are calls of trampolines each emitter writes at its
   first use: selfhost/arm64 with the clone trampoline (gen.xo
   trampolines, flag clone_ref: only programs with no C library use
   either), selfhost/llvm as naked functions (sched.xo unwind_fns). On
   x86_64 the save stores nothing and the resume traps: unwind.raise
   returns 0 at once there (stage 2).
3. **The C side**: with XO_NOLIBC, xo_rt.c's fault_raise calls the export
   `xo_x_unwind_fault` (cabi_nolibc.xo unwind_fault) and the C
   xo_personality and LSDA reader are not compiled; xo_nolibc.c loses its
   `_Unwind_*` stubs. The unwind tables name xo_personality; the static
   link defines it as cabi_nolibc.xo's export `xo_x_personality`
   (`-Wl,--defsym=xo_personality=xo_x_personality`) and asks for the
   `.eh_frame_hdr` table (`-Wl,--eh-frame-hdr`; the cc driver omits it
   for -static links). No C library unwinder function is linked.
4. **Scratch**: the unwinder's record (two register sets, the rule set,
   the CIE's rules, the remember stack; 20,480 bytes) is one mapping per
   thread, made at the thread's first fault and kept in thread local
   word 7 (sched.xo's table). Nothing in unwind.xo allocates or holds a
   counted value, so jumping away from its frame leaks nothing.

### Stages

- **Stage 1 (this decision, implemented 2026-10-14)**: XO_NOLIBC
  linux/arm64, arm64 and LLVM backends.
- **Stage 2**: XO_NOLIBC linux/amd64: the x86_64 trampolines (rbx, rbp,
  r12 to r15, rsp, the return address at the CFA; resume with rax and
  rdx) and DWARF register numbers per architecture in unwind.xo.
- **Stage 3**: the C library builds (glibc Linux and macOS) on the Xo
  unwinder, retiring xo_rt.c's libgcc path (fault_stop, fault_cleanup,
  the forced unwind, the C personality and LSDA reader): needs the
  shared objects' unwind tables (dl_iterate_phdr on Linux, a C library
  callback such as qsort can sit between a fault and its catch;
  macOS has compact unwind in `__unwind_info` and keeps libunwind, a
  platform library under decision 0195, so there only the personality
  routine would move).

## Consequences

- XO_NOLIBC programs run deferred code and landing pads on a fault and
  catch faults in tasks and `task.isolate`, as the C library builds do.
- xo_nolibc.c is 41 lines shorter (1,614 to 1,573); xo_rt.c gains a 15
  line nolibc fault_raise and two guards; unwind.xo is 748 lines of Xo
  (formatted, comments included). The libgcc path in xo_rt.c
  stays for the C library builds (stage 3).
- unwind.xo reads only what GNU ld's `--eh-frame-hdr` writes (a version 1
  header and a datarel sdata4 table); a link without the table, or a
  frame whose CFI uses DWARF expressions, ends the walk (the fault is
  reported, the program ends with status 70, as before).
- A fault from a signal handler cannot unwind past the kernel's signal
  frame (libgcc recognises the sigreturn trampoline; unwind.xo does not):
  it is reported. No runtime path raises a fault from a handler today.

## Alternatives

- **Keep libgcc's interface and implement `_Unwind_ForcedUnwind` and its
  accessors in Xo**, calling the C stop function and personality: needs
  C calls of five and six arguments from Xo (rt.call_c2 has two), and
  keeps the C personality. The chosen design keeps the protocol inside
  Xo; the personality is still an export with the libgcc signature.
- **Restore through rt.stack_switch** (a frame of saved registers built
  under the target's stack pointer): the frame would overlap the
  unwinder's own live frames when the landing frame is close above, and
  x0 cannot be set.
- **Walk .eh_frame linearly**: a static link with -nostdlib has no
  crtend terminator to stop at; the header's table is also O(log n).
