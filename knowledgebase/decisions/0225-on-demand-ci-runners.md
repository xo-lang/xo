# 0225: On demand CI on self-hosted EC2 runners

Status: Proposed (owner, 2026-10-14; stub claiming the number, filled in
as the work lands).

## Context

Agents verify heavy work through the lead's Mac (heavy.sh lock) and an
EC2 job queue driven by scripts on the Mac. The Mac's RAM limits how
many agents can verify at once, and the queue needs the Mac and a lead
session awake.

## Decision

GitHub Actions with self-hosted runners on the existing warm EC2
machines (imported into Terraform in infra/ci-runners, state in S3),
registered through a GitHub App. Every workflow is started by hand
(workflow_dispatch): no push, pull request, or schedule triggers. A
GitHub workflow_job webhook to a Lambda starts a stopped machine of the
job's label; machines stop themselves after 5 idle minutes. The full
tier runs on demand only.

## Consequences

To be filled in.

## Alternatives

To be filled in: ephemeral runners per job
(philips-labs/terraform-aws-github-runner), a polling Lambda, a Mac
runner, GitHub hosted runners.
