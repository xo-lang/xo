# A task started on a stored task group must not fail

Status: Anticipated (2026-10-07, task groups in the P1 checker, core 7.1)

Symptom: `c.tasks.spawn_background(fn() { refresh(c)? })` where `c.tasks`
is a `task.Group` field is XO0801, even though the same code on `os.tasks`
in `main` is accepted.

Cause: a failing group task fails the scope that created the group (core
7.1, decision 0032), and its error must convert to that scope's error type.
For a group read from a field or a parameter the creating scope is not
visible, so the checker cannot verify the conversion. P1 requires such tasks
not to fail unless their handle is bound and awaited, the same rule as for
scope handles received from elsewhere.

Fix: handle the error inside the task (`match refresh(c) { Ok(_) => ()
Err(e) => log.warn("refresh failed", {"err": e}) }`), or start the task where
the group's origin is visible (a local bound to `s.group()`, or `os.tasks`
in `main`). See reading 5 in `bench/results/check-corpus-v0.4-spec.md`.
