# One slow docker info under Mac load is not a dead daemon

Status: Observed (2026-11-17, xw-1, task #214)

## Symptom

At about 04:56 on 2026-11-17, at a Mac load of about 27, hangcheck.sh
reported `DOCKER-DEAD daemon did not answer docker info in 15 s`. xw
preflight reads that alert (check_docker), so it declared hosts 3 and 4
(containers on the Mac) down and sent azure-1's run to host 2. The same
`docker info` answered in 0 to 1 s 30 s later.

## Cause

hangcheck.sh judged the daemon on one `docker info` with a 15 s limit.
Under heavy load the Docker Desktop VM is descheduled for seconds at a
time (see docker-burst-stalls-on-loaded-mac.md), so one call can pass 15 s
while the daemon is healthy.

## Fix

hangcheck.sh tries `docker info` up to 3 times, `HANG_DOCKER_RETRY_S` (30)
seconds apart, each with `HANG_DOCKER_WAIT_S` (15), and reports
DOCKER-DEAD (no answer, or refuses) only when all 3 fail.
`scripts/lead/hangcheck_docker_test.sh` (test-heavy.sh scenario 20)
checks it with a stand-in docker that is slow or refuses once, then
answers.
