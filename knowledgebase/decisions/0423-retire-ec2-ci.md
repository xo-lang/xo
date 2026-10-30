# 0423: Retire the EC2 CI runners; heavy runs on the owner's offload hosts

Status: Accepted (owner, 2026-10-29). Supersedes 0225.

## Context

Decision 0225 ran heavy verification as GitHub Actions jobs on
self-hosted EC2 runners (infra/ci-runners), started by hand. On
2026-10-29 the owner provided two machines on the local network: a
Linux arm64 container and a 16-core x86-64 Windows machine with WSL2
(infrastructure/offload.md). They already run the full tier's gen, fuzz
and selfhost steps (arm64) and every amd64 golden natively (amd64), the
second in 33 s for the per-push sample against about 15 minutes in
Docker on the Mac. Only the Windows runner and the full selfhost tier
before a tag still needed EC2.

## Decision

The owner had the three remaining EC2 runners (xo-ci-amd64-big,
xo-ci-windows, xo-ci-arm64-big) terminated with their volumes and the
six runner registrations removed from GitHub (2026-10-29). The rest of
the infra/ci-runners stack (the register and autostart Lambdas, the
one minute sweep, the repository webhook, the IAM roles, the boot
scripts in the seed bucket, the webhook secret) is destroyed with
`terraform destroy` there (run by the owner); runners.json is empty. The seed bucket, the
Terraform state bucket (infra/state) and the xo-lang-ci GitHub App stay.

Heavy runs go to the offload hosts: `scripts/lead/offload.sh` (arm64),
`OFFLOAD_ENV=offload-amd64.env` and `scripts/lead/amd64-run.sh`
(linux/amd64), and native Windows runs on the same x86-64 machine
(to be scripted). `gh workflow run ci.yml` has no runner.

## Consequences

- No AWS compute cost for CI; the machines are the owner's.
- The offload hosts run one job at a time each; there is no autoscaling.
- The full selfhost tier (58 min on r8g.8xlarge) moves to an offload
  host before the next tag.
- Windows threads, signals and IOCP checks run on real Windows hardware
  instead of the xo-windows runner.
- `infra/ci-runners` stays in the repository: `terraform apply` there,
  with runners.json entries, rebuilds the EC2 CI if it is ever wanted.

## Alternatives

- Keep the stopped runners (about $30 a month of disks): rejected by the
  owner.
- Keep the Lambdas and webhook without runners: they would sweep every
  minute for nothing.
