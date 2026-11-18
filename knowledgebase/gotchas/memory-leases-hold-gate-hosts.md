# Memory leases on hosts 2 and 3 queue every gate

Status: Observed (2026-11-18)

Symptom: gates wait for hours on pass2 (irman 1h28m, peakfix and t326f
queued) while host 4 runs only no-lane evidence.

Cause: a memory lease (`OFF_MEM=1`, heavyd `-mem`) owns its whole lane.
#326's host-2-only llvm repro held all three pass2 slots, another memory
run held pass3, and offload-gate.sh never sent a plain gate to host 4
(whole-host runs only).

Fix (#345): offload-gate.sh sends a short gate (gatelib.sh `gate_short`:
no llvm, arm64, corpora-all, cmd, devllvm or fp step) to host 4 when
`gate_lanes_held` finds hosts 2 and 3 held whole by memory leases
(`OFFGATE_SHORT4=0` turns it off). A long single-host memory repro takes
`OFF_MEM_SLOTS=2` (heavyd `-mem=2`): it holds 2 of 3 slots and one plain
gate still fits beside it. heavyd picks up `-mem=N` after the lead
restarts it.
