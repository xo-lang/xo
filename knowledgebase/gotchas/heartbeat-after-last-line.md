# A heartbeat can land after a job's last line

Status: Observed (2026-11-17, task #202)

Symptom: `TestGateStepsShared` (scripts/lead/gatesteps_test.sh, "corpus
run: heartbeats while a slow check runs") failed on the linux/arm64
offload hosts and passed on the Mac: got `beatrun: heartbeat: go still
running, 3 s` where the check's own line was expected. It blocked other
agents' gates (Lang 1's pjfield, #189) that never touched scripts/lead.

Cause: the stub check slept 3 s and then printed, and beatrun.sh beat
every 1 s, so the third heartbeat and the check's end fell on the same
moment. On a loaded host the heartbeat sometimes came after the check's
line (1 of 40 runs under 8 CPU hogs in a Linux container), and the case
read the log's last line.

Fix: a test of a job that heartbeats reads the job's own lines (skip
`beatrun: heartbeat` lines), never `tail -1` of the log; and the stub
prints before it sleeps, so heartbeats always follow its line and an
order-dependent assertion fails every time instead of now and then.
A timing case that passes on the Mac and fails on a host is reproduced
in a Linux container under load (`docker run --cpus 4` with CPU hogs)
before blaming the host.
