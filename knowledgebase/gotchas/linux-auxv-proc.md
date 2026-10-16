# The unwinder's AT_PHDR from /proc/self/auxv: a fault escaped its isolate

Status: Observed (2026-10-16, linux/arm64 under valgrind, both native
backends); fixed 2026-10-16 (xo-4).

## Symptom

Under valgrind (callgrind or `--tool=none`) a native program's fault
inside `task.isolate` was not caught: it printed `fault: index ... at
...` and exited 70 from the first fault. About a quarter of runs with
valgrind's default scheduler, every run with `--fair-sched=yes`, never
natively (0 of 300) and never with main dd33dd13 (libgcc's unwinder).
It made callgrind's counts of bench/perf fault meaningless (owned: 150K
instructions, the program ended at its first fault).

## Cause

unwind.xo raise finds the program's .eh_frame_hdr through the program
headers at AT_PHDR, which linux.auxval read from /proc/self/auxv. Under
valgrind that file (valgrind's own copy of the client's vector) came
back from a later offset: the tail of the vector, AT_NULL, then its head
again, so the parse stopped at AT_NULL before AT_PHDR, eh_frame_hdr was
0, and raise returned "nothing catches" at once. glibc reads the vector
from the initial stack, where it is whole. The same dependence on /proc
would lose every fault where /proc is not mounted or not readable (a
chroot, a sandbox).

## Fix

process.xo start keeps the auxiliary vector's address when envp is the
initial stack's (right after argv's NULL ended array): linux.xo
auxv_init walks envp to its NULL and keeps the next word's address
(static word 12); auxval reads entries there and falls back to
/proc/self/auxv (auxval_proc) only without it (the C tests'
constructor). Test: golden rt_addrs's `rt.auxv` (kept, AT_PHDR equal to
/proc's, AT_PAGESZ set; 01 01 01 off Linux). Found with the debug words
of a scratch std copy: the failing raise always stopped at
`eh_frame_hdr() == 0`.
