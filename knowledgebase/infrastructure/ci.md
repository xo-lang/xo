# CI: on demand GitHub Actions on self-hosted EC2 runners

Decision 0225 (owner, 2026-10-14). Heavy verification (full package
tests, the selfhost merge tier, the fixed point, the full tier) runs on
linux/arm64 EC2 machines as GitHub Actions jobs, so it no longer takes
the Mac's RAM or needs a lead session awake. Nothing runs on push, pull
request, or a schedule: every run is started by hand, by an agent, or by
the owner. Last verified: 2026-10-14.

Contents: [Running CI](#running-ci), [Suites](#suites),
[How it works](#how-it-works), [Runners](#runners),
[AWS and GitHub inventory](#aws-and-github-inventory),
[Operations](#operations), [Measurements](#measurements),
[Cost](#cost), [Why it is built this way](#why-it-is-built-this-way),
[Not done yet](#not-done-yet).

## Running CI

```
git push origin BRANCH                                   # CI checks out from GitHub
gh workflow run ci.yml -f ref=BRANCH -f suite=auto       # merged with main by default
gh run list --workflow ci.yml --limit 5                  # run names: "ci SUITE REF [+main]"
gh run watch RUN_ID --exit-status                        # blocks until done; exit status is the run's
gh run view RUN_ID --log-failed                          # only the failing step's log
```

Inputs of `.github/workflows/ci.yml`:

| input | default | meaning |
|---|---|---|
| `ref` | `main` | branch, tag, or commit to test (push it first) |
| `suite` | `auto` | what to run, see [Suites](#suites) |
| `packages` | empty | for `suite=merge`: space separated packages (`./cmd/xo ./internal/llvm`); empty means the touched ones |
| `merge_main` | true | test REF merged with the current main, without pushing anything; a conflict fails in seconds and lists the files |
| `runner` | `xo-arm64` | runner label; `xo-arm64-big` for heavy runs (the full tier) |

Starting the same REF and suite again cancels the earlier run
(concurrency group `ci-REF-SUITE`). Inputs reach the shell through
environment variables, never pasted into the script. The checkout step is
`actions/checkout@v7` (Node 24; v4 printed a Node 20 deprecation warning).

The workflow file must exist on main for `gh workflow run` to find it;
`ref` then chooses which commit's tree is tested (the workflow
definition itself comes from main).

## Suites

`scripts/ci/suite.sh SUITE [PACKAGE...]` runs the same on any machine
with Go and clang, so a failing suite can be reproduced locally.

| suite | runs |
|---|---|
| `auto` | `short`; then the full tests of the Go packages with a changed `.go` file; `./internal/selfhost` (merge tier) when anything under `selfhost/` or `std/` changed; the fixed point when the change touches what the self build compiles (CLAUDE.md's list: `selfhost/` but not `selfhost/macho`, `selfhost/wasm`, or `_test.xo` files; `std/native/`; `std/llrt/`; `internal/xotool` xotool.go, chain.go, seed.go, lock_*.go; `scripts/xobuild/`; `scripts/seed-chain.txt`) |
| `short` | `go vet` and `go test -short` over every package but `runs/` |
| `vet` | `go vet` only |
| `merge` | `go test -timeout 60m` of `packages` (default: the touched ones) |
| `fixed-point` | `go run ./scripts/xobuild` (the checkout's xo, from the newest seed) then `scripts/bootstrap-xo.sh` (s1, s2, s3; s2 and s3 identical) |
| `full` | every package with `XO_SELFHOST_TIER=full`, `-timeout 120m` (job limit 150 minutes); on demand only, before tagging or for a change the merge tier does not exercise |

"Touched" is `git diff --name-only $XO_CI_BASE...HEAD` (merge base with
`origin/main` by default), so with `merge_main` on it is still the
branch's own change. A Go package counts when a changed `.go` file's
directory still has `.go` files.

What stays on the Mac: Mac only checks (Mach-O, code signing, the darwin
fixed point), runs that write files back into a worktree (golden
recording; CI returns only logs), and instruction count measurements.

## How it works

```
gh workflow run ──> GitHub queues job (runs-on: self-hosted, xo-arm64)
                         │ workflow_job webhook (HMAC signed)
                         ▼
             Lambda xo-ci-autostart ◄── EventBridge every minute (sweep)
                         │ ec2:StartInstances (only tag purpose=xo-ci)
                         ▼
      stopped runner machine boots ──> user data (cloud-init, every boot)
                         │ fetches s3://xo-seeds-ACCOUNT_ID/ci/boot/linux.sh
                         ▼
   boot script: 8 h cap, idle timer, toolchain, runner registration
                         │ aws lambda invoke xo-ci-register -> registration token
                         ▼
          runner service online ──> takes the job ──> job ends
                         │
   xo-ci-idle timer (every minute): no Runner.Worker for 5 minutes
                         │ stop runner service, check once more, poweroff
                         ▼
                  machine stopped (shutdown behavior: stop; disk and caches kept)
```

- **Autostart** (`infra/ci-runners/lambda/autostart.mjs`): on a
  `queued` webhook, and every minute, it lists the queued jobs of
  queued and in-progress runs, the runners (GitHub API), and the
  `purpose=xo-ci` machines (EC2). A job is covered by an idle online
  runner or a machine already booting; otherwise it starts a stopped
  machine whose `ci-labels` tag holds all of the job's `xo-*` labels,
  the one with the fewest labels first (the r8g.4xlarge before the big
  ones). If every fitting machine is busy the job waits for the first
  free runner. Two webhooks handled at once can start one machine too
  many; it stops itself after the idle time
  (gotchas/lambda-reserved-concurrency-account-minimum.md).
- **Registration** (`lambda/register.mjs`): a machine calls it at boot
  with its runner name; it answers whether GitHub still knows that
  runner and a fresh registration token. The GitHub App key stays in
  SSM, readable only by the Lambdas, so job code on a runner can at most
  register a runner, never administer the repository. When GitHub has
  removed the runner (offline over 14 days), the boot script registers it
  again with `--replace`.
- **Boot script** (`infra/ci-runners/boot/linux.sh`, a Terraform
  template): safe to rerun; installs only what is missing. Order: the 8
  hour lifetime cap per boot; the idle stop (installed before anything
  that can fail, and paused by `/run/xo-ci-booting` while the script
  runs); toolchain (dnf git clang llvm gcc make nodejs jq libicu, Go
  1.26.1 in /usr/local/go); the runner (latest release, in
  `/home/ec2-user/actions-runner`, a systemd service as ec2-user).
  Jobs get `PATH` with Go, `AWS_REGION`, and ec2-user's HOME, so
  `~/.cache/xo` (seeds, xo builds, backend cache) and Go's caches carry
  over between jobs. Log: `/var/log/xo-ci-boot.log`.
- **Idle stop** (`/usr/local/sbin/xo-ci-idle`, `xo-ci-idle.timer`): a
  stamp in `/run` is touched while `Runner.Worker` runs; after
  `idle_minutes` (5) without it, the runner service is stopped first and
  the machine powers off only if no job arrived in between.
- **Jobs**: each starts from a clean checkout (`actions/checkout`
  cleans the work tree, so `bin/xo` is restored from the shared xotool
  cache, not rebuilt). One runner per machine, so jobs on one machine
  never share the cache at once.

## Runners

| GitHub runner | instance | type | labels | role |
|---|---|---|---|---|
| xo-ci-arm64 | i-EXAMPLE06 | r8g.4xlarge (16 vCPU, 128 GB) | xo-arm64 | default jobs |
| xo-ci-arm64-big | i-EXAMPLE01 | r8g.8xlarge (32 vCPU, 256 GB) | xo-arm64-big, xo-arm64 | heavy jobs, overflow |
| xo-ci-arm64-big2 | i-EXAMPLE46 | r8g.8xlarge | xo-arm64-big, xo-arm64 | heavy jobs, overflow |
| xo-ci-arm64-big3 | i-EXAMPLE29 | r8g.8xlarge | xo-arm64-big, xo-arm64 | heavy jobs, overflow |

All four were the lead's warm machines (scripts/warm-run.sh), adopted
into Terraform on 2026-10-14 with their 120 GB disks and caches. Because
the big machines also carry `xo-arm64`, a default job can run on one
when it is already up (GitHub picks any idle runner with the labels; the
autostart only decides which stopped machine to start).

Not runners (still plain warm machines, tag `purpose=xo-warm-run`):
three c7i.8xlarge amd64 (i-EXAMPLE02, i-EXAMPLE56,
i-EXAMPLE08) and the Windows c7i.4xlarge (i-EXAMPLE47).

The list is `infra/ci-runners/runners.json`; change it with
`infra/ci-runners/runners.sh` (below), never by hand in the console.

## AWS and GitHub inventory

Account ACCOUNT_ID, us-east-1. Everything below is Terraform managed
(tag `project=xo-ci-runners`) except where noted.

| what | name | notes |
|---|---|---|
| Terraform state bucket | `xo-tfstate-ACCOUNT_ID` | infra/state; versioned (old versions kept 90 days), private, AES256, `prevent_destroy`, S3 lock files; keys `state/` and `ci-runners/` |
| runner machines | `xo-ci-*` | tags `purpose=xo-ci`, `ci-runner`, `ci-labels`, `ci-arch`; instance metadata tags enabled; IMDSv2 required |
| runner role | `xo-ci-runner` (+ instance profile) | s3 read on the seed bucket, s3 put on `dev/*`, invoke xo-ci-register |
| Lambda role | `xo-ci-lambda` | ssm:GetParameter on `/xo-ci/*`, ec2:DescribeInstances, ec2:StartInstances only on `purpose=xo-ci` |
| Lambdas | `xo-ci-register`, `xo-ci-autostart` | Node.js 22; code `infra/ci-runners/lambda/`; logs `/aws/lambda/<name>`, 14 days |
| function URL | xo-ci-autostart | auth NONE; the handler checks GitHub's `X-Hub-Signature-256` |
| schedule | EventBridge rule `xo-ci-autostart-sweep` | `rate(1 minute)`, input `{"sweep": true}` |
| boot script | `s3://xo-seeds-ACCOUNT_ID/ci/boot/linux.sh` | rendered from boot/linux.sh |
| webhook secret | SSM `/xo-ci/webhook-secret` (SecureString) | random, 48 characters |
| GitHub App | `xo-lang-ci`, id 5318159, installation 171424164 | NOT Terraform: made by `infra/ci-runners/github-app/create-app.py` (manifest flow); installed on josephspurrier/xo only; permissions administration write, actions read, webhooks write, metadata read |
| App credentials | SSM `/xo-ci/github-app/{app-id,slug,installation-id,private-key}` | NOT Terraform (written by create-app.py); the key is a SecureString and was never printed or written to disk |
| repository webhook | id 697891712 | event `workflow_job`, to the function URL |
| security group | `xo-ci-ssh` (sg-EXAMPLE01) | NOT Terraform (pre-existing, read as data); SSH only, runners need no inbound access |
| key pair | `xo-ci` (`~/.ssh/xo-ci.pem`) | NOT Terraform (pre-existing); for debugging over SSH |

The pre-existing role `xo-ci-seed-read` (seed bucket read) is no longer
on the runners; scripts/remote-*.sh still use it for their own
instances.

## Operations

All commands from the repository root, with the AWS CLI signed in to
the account, `gh` signed in as a repository admin, and Terraform 1.10 or
later (1.16.2 on the Mac).

```
infra/ci-runners/runners.sh list                          # machine state, GitHub status, busy or idle, labels
infra/ci-runners/runners.sh add arm64-2 r8g.4xlarge xo-arm64            # a new machine; registers at first boot
infra/ci-runners/runners.sh add arm64-big4 r8g.8xlarge xo-arm64-big,xo-arm64 us-east-1d
infra/ci-runners/runners.sh adopt amd64-big i-EXAMPLE02 xo-amd64  # an existing machine, disk kept
infra/ci-runners/runners.sh remove arm64-big3             # refuses while busy; deregisters, deletes machine and disk
infra/ci-runners/runners.sh start|stop NAME               # by hand
infra/ci-runners/runners.sh apply                         # terraform apply of runners.json as it is
```

`add`, `adopt`, `remove`, and `apply` show the plan and ask (`XO_TF_AUTO=1`
applies without asking). After any change to `runners.json`, commit it.
Adopting or changing user data starts a stopped machine once
(gotchas/terraform-user-data-starts-stopped-instance.md); it stops again
after the idle time.

- **Changing the boot script or a Lambda**: edit
  `infra/ci-runners/boot/linux.sh` or `lambda/*.mjs`, then
  `cd infra/ci-runners && terraform apply`. The boot script takes effect
  at each machine's next boot; to apply it now on a running machine:
  `ssh ec2-user@IP 'sudo bash -c "aws s3 cp s3://xo-seeds-ACCOUNT_ID/ci/boot/linux.sh /usr/local/sbin/xo-ci-boot.sh --region us-east-1 && AWS_REGION=us-east-1 bash /usr/local/sbin/xo-ci-boot.sh"'`.
  Template syntax: Terraform interpolates `${name}`; write shell
  variables as `$NAME` (or `$${NAME}`), never `${NAME}`.
- **Changing the workflow or suites**: `.github/workflows/ci.yml` and
  `scripts/ci/suite.sh`, pushed to main (the workflow definition always
  comes from main).
- **A job waits in "queued"**: `runners.sh list`; then the autostart
  log (`aws logs tail /aws/lambda/xo-ci-autostart --since 15m`), which
  prints `{"jobs": [...], "start": [...]}` whenever it starts anything.
  Common causes: every fitting machine busy (add one), a label no
  runner has (check `runs-on`), a machine still `stopping` (the sweep
  starts it once it is stopped), the vCPU quota (160 on demand standard
  vCPUs: the four arm64 runners use 112 when all run).
- **A machine is up but its runner is offline**: SSH in, `sudo tail -50
  /var/log/xo-ci-boot.log`, `systemctl status "$(cat
  ~/actions-runner/.service)"`.
- **A machine never stops**: `journalctl -u xo-ci-idle`; the 8 hour cap
  powers it off regardless.
- **Webhook deliveries**: GitHub, repository Settings, Webhooks (or
  `gh api repos/josephspurrier/xo/hooks/697891712/deliveries`). A missed
  delivery only delays a start until the next sweep.
- **Rotating the App key**: GitHub, Settings, Developer settings, GitHub
  Apps, xo-lang-ci, generate a private key; store it with `aws ssm
  put-parameter --name /xo-ci/github-app/private-key --type
  SecureString --overwrite --value file://KEY.pem`, delete the file and
  the old key on GitHub; then `terraform apply` (the github provider
  reads it). The Lambdas pick it up on their next cold start.
- **Recreating the App**: `python3 -I
  infra/ci-runners/github-app/create-app.py`, open
  http://localhost:8975, confirm, install on josephspurrier/xo only.
  It overwrites the SSM parameters; then `terraform apply`.
- **Terraform state**: `cd infra/state && terraform plan` should show no
  changes; the bucket cannot be destroyed by Terraform.
- **Tearing everything down**: `runners.sh remove` each runner (or
  `terraform destroy` in infra/ci-runners, which deletes the machines and
  their disks), then delete the App on GitHub and `/xo-ci/github-app/*`
  in SSM.

Safety (CLAUDE.md): never stop, signal, or kill processes by name or
pattern on a runner or on the Mac; the idle script only looks for
`Runner.Worker` with `pgrep`, it never kills by pattern.

## Measurements

From the first runs on main, 2026-10-14
(runs 38595689932, 38596682112, 38597216716, 38597341138):

| what | time |
|---|---|
| `short` (vet and `go test -short`, every package) on xo-ci-arm64 | 51 s step, 81 s queued to done |
| stopped machine to job running (webhook, start, boot, runner online) | 75 s |
| idle stop after the last job | 5 min 24 s |
| `fixed-point` on xo-ci-arm64-big | 2 min 24 s job: s1 36 s, s2 36 s, s3 24 s; s2 and s3 assembly (297,336,303 bytes) and binaries (53,485,152 bytes) identical |

Earlier figures for comparison (from the knowledgebase): one self build
59.00 s on the Mac (M4 Pro, 12 cores, 24 GB) against 87.84 s at 8 jobs
and 49.83 s at 16 jobs on r8g.8xlarge (toolchain.md, decision 0220); one
seed chain hop 311 s on the Mac against 411 s on r8g.8xlarge (decision
0216). Core for core the Mac is about 1.3 to 1.5 times faster, but a 16
job self build peaks at 19.86 GB, most of the Mac's RAM, and the full
tier (peak about 157 GB) does not fit on it. The full tier's 58 minutes
(3,474 and 3,490 s cold, 2026-10-11 and 2026-10-12) predates parallel
compilation and the backend cache; it has not been remeasured on CI.

## Cost

- Machines, on demand only while running: r8g.4xlarge about $0.94 an
  hour, r8g.8xlarge about $1.89. A `short` run costs about 2 cents; a
  full tier run about $2.
- Stopped disks: 120 GB gp3 each at $0.08 per GB-month, about $9.60 a
  month per machine; about $38 for the four runners, $77 with the four
  other warm machines.
- Lambdas, EventBridge, SSM, S3 state: effectively free at this volume.
- GitHub: nothing. GitHub announced a $0.002 a minute platform fee for
  self-hosted runners on private repositories from 2026-03-01, then
  postponed it in December 2025 to "re-evaluate our approach"; no newer
  notice was found on 2026-10-14. If it returns: about $0.12 per hour of
  jobs, counted against the plan's included minutes first. Public
  repositories were always exempt.
- Account cleanup on 2026-10-14 (owner request): the unrelated
  `a53x-a17-gsi-build` project (c6i.8xlarge, its 40 GB and 350 GB disks,
  security group, key pair) was deleted, about $31 a month. The
  `truewares-web-app` resources (RDS, Fargate, VPC link) belong to
  another project: never touch them.

## Why it is built this way

Full record: decisions/0225-on-demand-ci-runners.md. In short:

- On demand only (owner): keeps development moving, no runs on work in
  progress, flexible around merge conflicts; `merge_main` answers "will
  this merge and pass" before anyone merges.
- The full tier on demand, rarely before 1.0 (owner); no nightly
  schedule.
- Adopted warm machines (owner) over a fresh machine per job: the
  caches survive, a self build is not cold.
- A GitHub App (owner) over a personal token: no expiry, permissions on
  this repository only.
- S3 state (owner): any session can run `runners.sh`.
- Webhook plus a one minute sweep over polling alone: starts in about a
  minute, and a lost webhook costs at most a minute.
- The machines stop themselves: no outside system can stop a machine in
  the middle of a job, and nothing depends on the Mac.
- It replaces the lead's Mac-side EC2 queue (ec2q-runner4, warm-run2,
  stop-idle-loop2): retired 2026-10-14. Never run them against the
  `xo-ci-*` machines; they look for `Name=xo-warm` and would create new
  instances.

## Not done yet

1. Time the full tier on CI: `gh workflow run ci.yml -f suite=full -f
   runner=xo-arm64-big`.
2. The first `suite=auto` run on a real agent branch (auto's path rules
   are untested on a branch with changes).
3. amd64 runners (best effort): `runners.sh adopt` the three c7i.8xlarge
   machines with label `xo-amd64`, add the choice to `ci.yml`'s `runner`
   input; the fixed point there uses the LLVM backend.
4. A Windows runner (best effort): a PowerShell boot script
   (`boot/windows.ps1`) with Defender exclusions (see
   scripts/remote-windows.sh), user data `<powershell>` with
   `<persist>true</persist>`.
5. Decide whether four big machines are needed; each stopped one costs
   about $9.60 a month.
