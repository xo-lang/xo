# Offload job directories pile up on /work

Observed 2026-11-15 (Xo Lang 2, host 2 cleanup for the lead). Status: Observed.

**Symptom.** Host 2's /work fell to 16 GB free, under irman's 25 GB floor,
so the IR manifest build was skipped there; offload.sh's prune ran before
every job and freed almost nothing ("pruned least recently used cache
files, now 19 GB").

**Cause.** offload-prune.sh pruned only the named caches and never
touched /work job directories, and nothing else removed a gate's,
stdproven run's or tier's directory after its job: 599 of them, idle 1 to
13 days, held 6 GB. Agents' named scratch directories held more (18 GB
of Lang 1's, recreatable). And `/` on host 2 is a container overlay on
the same disk as /work: about 50 GB in use there belongs to the
container host (images, other containers, build cache), which this user
cannot see or clean.

**Fix.** offload-prune.sh's job_dirs pass removes /work/gate*, spl* and
tier* directories (with their .logs) idle 24 hours that no process works
in, after the caches, while /work is still under its target. Idle means
no file read and nothing modified: a directory's atime moves whenever
find reads it under relatime, so only files' atimes count. The container
host's share is the owner's (OWNER-MORNING).
