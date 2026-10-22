# Native working directory on macOS, 2026-10-22

Item: fsproc cwd, native at 0.14x of the Go backend (go-backend-gap-2026-10-22.md).
Program: bench/perf/fsproc, workload cwd (20,000 `os.fs.cwd().case_sensitive()`).

Cause (`XO_PROF=cpu`): 90% of the samples were `open$NOCANCEL`. libc's
getcwd on macOS opens `.` and asks the kernel for its path with
fcntl(F_GETPATH); 3.3 G instructions retired, 0.21 s system time. Go's
os.Getwd tries $PWD first and checks it with two stat calls.

Fix (std/native/llrt/darwin.xo): darwin.Stat gains dev and ino;
getcwd returns $PWD when it is absolute and names the same directory as
`.`, else calls getcwd as before. Linux already uses the system call
directly and Windows does not consult $PWD.

`xo bench --ab`, 10 rounds, before (A) against after (B):

| metric | before | after | B / A |
|---|---|---|---|
| wall_ms | 227.3 | 35.9 | 0.158 (0.143 to 0.164) |
| cpu_ms | 220 | 20 | 0.091 |
| instructions | 3,372 M | 525 M | 0.156 (0.155 to 0.156) |

Go row (`xo bench --ab` native before against the Go backend): wall 34.5
ms, instructions 538 M. Native after is 35.9 ms and 525 M, level with Go.
