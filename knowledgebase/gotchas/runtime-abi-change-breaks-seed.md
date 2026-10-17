# A runtime ABI change crashes the self build's first stage

Status: Observed 2026-10-16 (xo-4, spawn's closure as two words).

## Symptom

After changing what std/native/llrt's task_spawn takes (a closure's two
words instead of a list of one) and the SSA builder that calls it, the
xo built by `go run ./scripts/xobuild` (seeded by the previous xo)
died with SIGBUS (exit 138) on every build, printing nothing.

## Cause

The seed compiles the new compiler's sources against the worktree's
std. The seed's own code generator still emits the old call (a list of
one) into the new runtime, which reads it as code and environment: the
new xo's own tasks (the compiler runs passes in parallel) jump to a
list's address. The same holds for scripts/bootstrap-xo.sh's s1 stage
and any release N-1 seed (decision 0216).

## Fix

Keep the old entry point beside the new one until a seed knows the new
one: rt.task_spawn (the list of one) stays and rt.task_spawn_fn is the
new name, the task records which form it got (tasks.xo T_TWO). Remove
the old one after the next seed cut (a minor version, owner approved).
The alternative is cutting a seed first (gotchas
seed-older-than-intrinsics.md), which needs the owner.
