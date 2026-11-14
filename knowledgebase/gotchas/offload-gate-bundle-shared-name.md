# offload-gate: two runs of one tree shared a bundle name

Symptom: an offload-gate rerun failed at once with "offload-gate: the
bundle did not fetch" (Lang 2's N2 seedbuild on host 1, 2026-11-13).

Cause: the bundle on the host was /work/in/gate-WORKTREE-REV.bundle, the
same for every run of the same tree. A stopped run whose lease had already
started still ran its cleanup, which removes its bundle, and that removed
the rerun's freshly copied bundle of the same name.

Fix: scripts/lead/offload-gate.sh names the bundle
gate-WORKTREE-REV-OFF_RUNID.bundle (OFF_RUNID from the caller, else a fresh
id, exported so a reroute keeps it), so a run's cleanup removes only its own.

Rule: anything a run puts on a shared host and later removes carries that
run's id in its name, not only the tree's.
