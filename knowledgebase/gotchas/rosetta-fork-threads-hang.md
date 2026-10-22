# Many threads forking at once hang under Docker Desktop's Rosetta

Status: Observed (2026-10-22).

## Symptom

The golden proc_children_load (16 tasks each running 25 `sh -c`
children, plus 4 spinning tasks) hangs in the Docker amd64 goldens
(scripts/docker-goldens.sh ... amd64) on the Mac, with zombie `sh`
children under it. Lib1 saw it with the xo of decision 0286's step 3
and with the one before it.

## Cause

Docker Desktop on Apple silicon runs linux/amd64 containers through
Rosetta (`/run/rosetta/rosetta` is mapped into every amd64 process),
not qemu. In the hung process 15 of 16 threads sit in the host
kernel's `rt_mutex_schedule` (FUTEX_LOCK_PI, host syscall 98): those
are Rosetta's own locks; Xo's runtime only uses FUTEX_WAIT and
FUTEX_WAKE. The zombies are children that ran and exited; the parent
is stuck inside Rosetta before it can reap them.

Measured on 2026-10-22 with an xo of 8515d1a35 (native amd64 backend):

- Rosetta, the golden: hangs 3 of 3 (after 0 to 9 children).
- Rosetta, 16 tasks forking, no spinning tasks: hangs 2 of 2.
- Rosetta, one task forking 25 children, or the spinning tasks alone:
  passes 2 of 2 each.
- The same amd64 binary under qemu-x86_64 on linux/arm64 (Docker,
  `apt-get install qemu-user`): passes 3 of 3.
- linux/arm64 natively (backend arm64, the same spawn_linux code in
  std/native/llrt proc.xo: clone(SIGCHLD), execve): passes 5 of 5.

So concurrent fork from many threads trips Rosetta, not Xo's child
reaping. Real linux/amd64 hardware was not available (CI off) to
confirm there directly.

## Fix

scripts/linux-goldens.sh skips the goldens listed in `rosetta_hangs`
(proc_children_load) when `/run/rosetta` is in its own maps, writing
`skip NAME (hangs under Rosetta)` to summary.txt. Native linux/arm64,
qemu and real hardware still run it. To run it emulated anyway, use
qemu-x86_64 in an arm64 container as above. If a real linux/amd64 run
ever hangs on it, this gotcha is wrong: remove the skip and chase
spawn_linux.
