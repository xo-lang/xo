# 0225: On demand CI on self-hosted EC2 runners

Status: Accepted (owner, 2026-10-14), stage 1 implemented: one runner
(xo-ci-arm64, r8g.4xlarge) ran `ci short main` in 51 s on 2026-10-14;
the other warm machines are adopted next, then the Mac queue scripts
retire. How to run: infrastructure/ci.md; machines: infra/ci-runners.

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

Details:

- The owner chose a GitHub App over a personal token (no expiry,
  permissions only on this repository), adopting the existing machines
  over fresh ones (their caches survive), and S3 state over local state
  (infra/state, bucket xo-tfstate-ACCOUNT_ID).
- The App key lives only in SSM and the Lambdas: a machine asks
  xo-ci-register for a registration token at boot, so job code on a
  runner can at most register a runner, not administer the repository.
- `auto` encodes CLAUDE.md's test policy (short; touched packages'
  full tests; internal/selfhost's merge tier for selfhost/ or std/;
  the fixed point for what the self build compiles) in
  scripts/ci/suite.sh, so the policy has one executable home.
- `merge_main` (default on) tests a branch merged with the current main
  without pushing anything, so a merge conflict or a semantic clash
  shows before anyone merges.

## Consequences

- Heavy verification leaves the Mac: its RAM no longer limits how many
  agents can verify at once, and nothing needs the Mac or a lead session
  awake. The Mac keeps Mac only checks and golden recording.
- One interface for agents (`gh workflow run`, `gh run watch
  --exit-status`, `gh run view --log-failed`) replaces the queue
  scripts and their UPDATE notes.
- Concurrency equals the number of runners with a label; adding one is
  `runners.sh add` (a few minutes, first build cold).
- Runner machines keep state between jobs (the warm caches); a job
  starts from a clean checkout, and the user cache is shared.
- GitHub removes a runner offline for 14 days; the boot script
  registers it again.
- Cost: machine hours while running and the stopped disks; the Lambdas
  and webhook are free at this volume; GitHub charges nothing for
  self-hosted runners today (its $0.002 a minute fee was postponed in
  December 2025; if it returns, about $0.12 an hour of jobs).

## Alternatives

- Runs on every push or pull request: rejected by the owner; on demand
  keeps development moving and avoids runs on work in progress.
- Ephemeral runners, one fresh machine per job
  (philips-labs/terraform-aws-github-runner): every job starts cold
  (about 6 minutes for a self build), losing the warm caches.
- A Lambda polling GitHub instead of the webhook: kept only as the one
  minute sweep that catches missed webhooks; alone it adds up to a
  minute to every start.
- This Mac as a runner: it competes with the agents for the same RAM.
- GitHub hosted runners: more per minute than the EC2 machines and no
  32 vCPU, 256 GB arm64 size.
- A nightly full tier: the owner wants it on demand only, rarely before
  1.0.
