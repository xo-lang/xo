# heavyd lane slots above the env file's OFF_SLOTS: jobs wait out their -max on the host lock

Status: Observed 2026-11-18.

## Symptom

On the amd64 host, leases started by heavyd ran to their `-max` with no
output: the 0.54.0 seed placement (a job of seconds on every other host)
was stopped at 20 minutes, rc 124, and two jobs raised 151 minute hang
alerts. The host itself answered ssh normally. On the Mac the job's
offload.sh had an ssh to the host running `wsl -e bash -l job.sh`, and
nothing came back.

## Cause

heavyd-lanes.conf raised the amd64 lane to 6 slots on 2026-11-17 (ADR
0708), but `offload-amd64.env` kept `OFF_SLOTS=2`. heavyd started 6
leases; the job wrapper inside WSL takes one of `OFF_SLOTS` flock slots
(`~/xo-ci/offload.lock.N`), so 4 of the 6 sat on the lock, their `-max`
counting from the lease, until heavyd stopped them.

## Fix

Keep the lane's SLOTS and the env file's OFF_SLOTS equal: the lead set
`OFF_SLOTS=6` in offload-amd64.env (2026-11-18 12:20; a copy of the old
file kept in notes/lead/), and the seed placement then took seconds. When
changing a lane's slots in heavyd-lanes.conf, change the env file in the
same step. A guard that alerts on a mismatch is xw-1's task.

## Related: seed placement on a lane that drains for an exclusive

The same day the seed placement also waited 2h on amd64 while Lang 2's
exclusive timing lease drained the lane behind a 4 hour plain run (the
starve rule held every other lease). 38076d31d lets a plain lease with a
`-max` that ends by the running lease's limit start beside a starved
exclusive. Place the seed host by host (`SEED_PLACE_HOSTS`), the drained
host last, so the other hosts do not wait behind it.
