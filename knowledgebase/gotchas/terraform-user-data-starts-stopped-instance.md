# Changing user data with Terraform left a stopped instance running

Status: Observed (2026-10-14).

Symptom: the stopped warm machine i-EXAMPLE06 was running after
the first infra/ci-runners apply (the in-place update of its user data,
role, tags, and metadata options took 51 s), and its boot script had
already run.

Cause: AWS changes an instance's user data only while it is stopped;
the Terraform AWS provider (5.x) stops the instance, changes it, and
starts it again, whatever state it was in before.

The same applies to a running machine: the plan shows its public IP as
"known after apply", the sign of the stop and start. Adopting a machine
someone is using (the Windows warm machine on 2026-10-14) interrupts
their work, so ask its user first and apply when it is free.

Fix: expected now; the machine stops itself after the idle time. When
adopting a machine (runners.sh adopt), expect it to boot once and
register. The boot script itself lives in S3, so changing it never
touches user data.
