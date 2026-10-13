# remote-full.sh said "terminating" but the instance kept running

Status: Observed (2026-10-13), fixed.

Symptom: batch 5's log ended with `remote-full: terminating
i-EXAMPLE14`, yet the on demand r8g.8xlarge was still running
over an hour later (the lead terminated it by its id).

Cause: cleanup() ran `aws ec2 terminate-instances ... || true` once and
never checked the result, so a failed call (a network or API error) was
silent. The instance's own shutdown timer (300 minutes) and the lead's
watchdog loop would have stopped it eventually.

Fix: cleanup() retries the terminate up to five times and confirms the
state is shutting-down or terminated, logging each attempt. When reading
a run's log, look for `instance <id> shutting-down` or `terminated`.
