# infra/ci-aws: the Xo CI matrix on real Linux machines

Ephemeral EC2 machines that run the Xo test jobs on linux/arm64 and
linux/amd64 hardware, then are destroyed. Results of the first run:
`bench/results/ci-aws-2026-10-09.md`.

## What it creates

Only these, all tagged `project=xo-ci` and named `xo-ci-*`:

- one EC2 key pair `xo-ci-key` (an ED25519 key from the `tls` provider; the
  private half is written to `xo-ci-key.pem`, mode 0600, gitignored, and
  deleted after destroy),
- one security group `xo-ci-ssh` in the default VPC (ingress tcp 22 from
  `var.ssh_cidr` only, egress all),
- one instance per entry in `var.instances` (default `c7g.4xlarge` arm64
  and `c7i.4xlarge` amd64), Canonical's Ubuntu 24.04 AMI, 60 GB gp3 root
  deleted on termination.

The default VPC, its subnets, the instance type offerings, and the AMIs are
read through data sources. The provider refuses any account other than
`var.allowed_account` (ACCOUNT_ID). State is local (`terraform.tfstate`,
gitignored).

## Run

```
infra/ci-aws/ci.sh                 # all jobs
infra/ci-aws/ci.sh sanitizers      # some jobs: short full race sanitizers
```

Requires terraform, the AWS CLI with credentials for the allowed account,
rsync, ssh, and python3 on the host. `ci.sh` checks the account, applies,
rsyncs the working tree (without `.git/`, `runs/`, `.tools/`, `.claude/`),
runs `remote-setup.sh` (Go at the host's version from go.dev, clang, lld,
llvm, build-essential), runs `remote-jobs.sh` on every machine in parallel,
copies the logs to `logs/<timestamp>/<arch>/` (gitignored), and in an EXIT
trap runs `terraform destroy -auto-approve` and `verify-teardown.sh`
(every list it prints must be `[]`).

Jobs (each with its own cold `GOCACHE`, like separate CI runners):

| Job | Command |
|---|---|
| short | `go vet $PKGS && go test -short $PKGS` |
| full | `go test $PKGS` |
| race | `go test -race ./std/rt` |
| sanitizers | `go test -v -run 'Runtime\|GoldenThreads\|GoldenTasks\|Deadlock\|ASan\|NativeSignal' ./internal/llvm` |

`PKGS` is `go list -e ./... | grep -v /runs/`.

If a run is interrupted so hard that the trap does not fire, run
`terraform -chdir=infra/ci-aws destroy -auto-approve` and then
`infra/ci-aws/verify-teardown.sh`.
