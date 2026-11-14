# Stopping offload.sh on the Mac leaves its job on the host

Status: Observed (2026-10-30; updated 2026-11-14: offload.sh's trap now stops the host side)

**Symptom.** A hung linux/arm64 offload job (xo-lang-4's w47-sigs, a
cargo build of a wasi:http component) was stopped through the harness on
the Mac after 78 minutes. The local offload.sh and its ssh were gone,
but on the host the job's `bash -c` (parent 1, an orphan), its
`su - xo -c` and the lock wrapper lived on, still holding a ticket in
/work/offload.q. Every job queued behind it kept waiting (11 jobs and
the lead's tier at the time).

**Cause.** offload.sh runs the job through `ssh ... "bash -c ..."` with
no pseudo-terminal. When the local ssh dies, sshd closes the channel but
sends the remote command no signal; a job that writes nothing (here a
waiter in the ticket loop) never hits SIGPIPE, so it runs on. The ticket
counts as live while its wrapper's PID is alive.

**Fix.** After stopping a host job locally, check the host and stop the
remote side by its exact PIDs. Use a direct ssh with offload.env's
values, not offload.sh, which queues behind the same line. First check
that the wrapper's command line names your job's upload and that the
`su` and `bash -c` above it are its parents, then `kill PID...`; never
by name. Then remove its /tmp/off.*, /tmp/offw.* and /work/in/ files.
xo-lang-4's script: ~/xo-lead/notes/xo-lang-4/scripts/host-stop-w47.sh.
A remote job with output also gets a `timeout` on its long steps (cargo
under `timeout 600`), so a hang ends on its own. A fix in offload.sh
(`ssh -tt`, so sshd hangs up the remote side) is the lead's to make.

**Update (Observed 2026-11-14).** offload.sh now stops the host side
itself. Xo Lang 2 stopped its own 8 hour stdproven fuzz (l2-cfuzz2, started
through stdproven-offload.sh, leased on host 2) with a harness TaskStop of
the Mac task. offload.sh's trap ran, logged `offload.sh: stopped here;
stopping run <id> on the host` and `offload: stopped run <id> (still
there: 0)`, and heavy.sh --status showed the lease gone from host 2's lane
within 20 s. So a TaskStop of the session's own task is how to stop a
started offload job: a cancel file (~/xo-lead/locks/cancel/NAME) only
drops a lease that has not started. Check both log lines. If `still
there` is not 0, use the manual Fix above. Rerun under a new name, since a
stopped run with the same name can delete a rerun's checkout
(offload-gate-stopped-run-deletes-rerun.md). Stopping a chain script by
its PID still orphans the offload-gate it started, which keeps its lease.
Stop the task instead, so the trap runs.
