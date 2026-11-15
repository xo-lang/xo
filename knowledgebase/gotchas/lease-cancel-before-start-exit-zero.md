# A lease cancelled before it started exited 0

Status: Observed (2026-11-15, Xo Lang 2).

Symptom: a chain `offload-gate.sh WT repo; r=$?; [ $r = 0 ] && pushq.sh WT --gated`
started pushq although the gate never ran. offload-gate's log ended with
"heavyd lease: NAME cancelled before it started" and "gate rc=0", and its
offload-gate-last record held rc 0 for that HEAD, which pushq --gated
accepts. The push was stopped (TaskStop of the chain) before it landed.

Cause: heavyd's lease client (lanes.go) and heavy.sh's client (client.go)
returned 0 when a cancel file arrived before the job started: "dropped"
and "passed" were the same exit code. The cancel had been sent to move a
gate that offload-gate had rerouted to host 3 off that host.

Fix: both return LeaseExitCancelled (130) for a cancel before the start,
as they already did for a cancel while running (lanes_test.go checks it).
Until a host has the new heavyd, never chain a push on a gate's exit code
alone after cancelling its lease: read the log for "cancelled before it
started".
