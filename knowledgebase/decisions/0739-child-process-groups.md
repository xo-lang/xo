# 0739: Child process groups

Status: Accepted, implemented (2026-11-21) on the Go backend, arm64 and
LLVM (owner answer xw #261, option 1; task #660).

## Context

xw-1's runner (task #600) starts jobs as children (`sh -c CMD`) and must
stop a whole job, including the processes the shell started. `Child.kill`
signals only the child's own pid, so a `sleep` or a build the shell
started lives on after its parent is killed, holding the job's pipes open.
Decision 0190 lets `Proc.kill` signal other processes but refuses a pid of
0 or less, so a program cannot signal a process group by number: that
refusal keeps an Xo program from signalling groups it did not start (its
own, or a guessed one), and it stays.

The owner chose option 1 of xw #261: a group the program starts, and a
signal to that group only.

## Decision

1. **`Cmd.new_group() -> Cmd`** marks the command to start in a new
   process group whose id is the child's pid. darwin sets it in the spawn
   attributes (`posix_spawnattr_setpgroup(at, 0)` with
   `POSIX_SPAWN_SETPGROUP`); linux calls `setpgid(0, 0)` in the forked
   child before exec, and the parent's exec pipe wait means the group
   exists when `spawn` returns; the Go backend sets
   `SysProcAttr.Setpgid`.
2. **`Child.kill_group(signal: Str = "TERM") -> () ! ProcErr uses proc`**
   sends the signal to the group (`kill(-pid, sig)`). It errors with
   `ProcErr.Io` when the child was not started with `new_group` ("the
   child was not started with new_group"), when it did not start, for an
   unknown signal name, and on systems without process groups (Windows:
   "process groups are not supported on this system"). The signal names
   are `Child.kill`'s.
3. **The group stays signalable after `wait`.** Other members may still
   run after the leader exits; signalling them then is the point. The
   pgid can be reused only once every member has exited, which is the
   same pid reuse risk `Child.kill` accepts after `wait`.
4. **No new capability.** `kill_group` needs `uses proc`, like
   `Child.kill`: the group is one the program started, so attenuation
   needs no separate listing.
5. The native spawn spec carries the request as flag bit 16; bit 8 is
   procspec's "environment final" marker, set on every spec.
6. The fake proc layer (tests, decision 0161) records `"NAME group"` for
   a `kill_group`, beside `Child.kill`'s `"NAME"`.

## Consequences

- A runner can stop a whole job: start it with `new_group()`, then
  `kill_group("TERM")`, later `kill_group("KILL")`.
- With `inherit`, a group child is not the terminal's foreground group,
  so a terminal's Ctrl-C does not reach it; programs that want Ctrl-C to
  reach the job forward the signal themselves.
- Windows returns an error rather than approximating with job objects; a
  job-object version is a separate decision.
- The golden `proc_group` covers both native backends and the Go backend;
  it is listed as unsupported on wasip1 and wasip2 (no processes).

## Alternatives

- **Let `Proc.kill` take a negative pid** (option 2 of xw #261): any
  group, including ones the program did not start; rejected, it undoes
  0190's refusal.
- **Kill the descendants by walking the process table**: racy (a child
  forks while the walk runs) and different on every system.
- **Make every child a group leader**: changes terminal signal delivery
  for every existing program that uses `inherit`.
