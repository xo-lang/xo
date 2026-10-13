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
The on demand vCPU quota (Running On-Demand Standard instances,
L-1216C47A) was raised from 32 to 96 on 2026-10-13: three 32 vCPU
instances at once; past that, `VcpuLimitExceeded` means wait and retry.
