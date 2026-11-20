# An offload gate's log is silent through a long step

Status: Observed (2026-11-19, xw-1)

## Symptom

An offload gate's log on the Mac (notes/AGENT/gate-*.log) shows the
header and `step precheck ended`, then nothing for over an hour; heavyd
marks the lease STALLED. It looks hung. On 2026-11-19 xw1-425mon was
cancelled on host 4 after 2h34m for this, and its rerun on host 3 was
healthy all along: the binary selection's builds took about 55 minutes,
then 774 goldens passed while the log stayed silent.

## Cause

offload-gate-remote.sh wrote gate.sh's output to gate.out on the host and
streamed back only a line as each step ended. A gate of one long step
(llvm: 5,785 to 7,025 s) wrote no line until it ended. gate.sh's 600 s
heartbeat lines were in gate.out, unseen; and they counted STEP.txt,
which a go test -json step writes only at its end, so each said "no new
lines". Since #449 heavyd also stops a whole-lane lease quiet for twice
its stall mark (90 minutes for llvm), which would have killed such a
gate.

## Fix

0361ea14b streams the heartbeat lines (`offload-gate: beat: gate.sh:
STEP running ...`); the commit after it makes pool_beat count
STEP.json while STEP.txt is empty. Before cancelling a silent gate on a container host (hosts 3 and
4), look at the host: `docker exec CONTAINER ls -la /work/gateNAME.logs/`
(STEP.json growing means alive) and its process tree with `ps -eo
pid,ppid,etime,args --forest`.
