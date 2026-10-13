# EC2 spot instances reclaimed mid-run, and no spot capacity

Status: Observed (2026-10-12 and 2026-10-13).

Symptom: a `scripts/remote-full.sh` run ends early with `Connection to
... closed by remote host` and few or no `ok` lines; the instance's
state reason is `Server.SpotInstanceTermination`. Or the launch fails
with `InsufficientInstanceCapacity` for r8g.8xlarge and r7g.8xlarge.

Cause: spot capacity in us-east-1 for large Graviton memory types was
short; three runs (one after 6 minutes, one after 27 of 44 packages)
were reclaimed, and the full selfhost tier needs about 45 to 60
minutes uninterrupted.

Fix: the scripts launch on demand only (owner, 2026-10-13).
On demand vCPU quota was 32 (request to 96 pending on 2026-10-13), so
two on demand runs of 16 or more vCPUs each can hit `VcpuLimitExceeded`;
run them one after another, or use a smaller type with XO_REMOTE_TYPE.
