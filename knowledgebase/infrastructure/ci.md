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
gh run watch RUN_ID --exit-status --interval 10          # blocks until done; exit status is the run's; poll every 10 s (owner 2026-10-14)
gh run view RUN_ID --log-failed                          # only the failing step's log
```

Inputs of `.github/workflows/ci.yml`:

| input | default | meaning |
|---|---|---|
| `ref` | `main` | branch, tag, or commit to test (push it first) |
| `suite` | `auto` | what to run, see [Suites](#suites); `auto` fans out into parallel jobs |
| `packages` | empty | `auto`: packages added to the full tests (`./internal/progen`); `merge`: the packages (empty: the touched ones); `measure`: the `bench/perf` directory (`testworld`) |
| `merge_main` | true | test REF merged with the current main, without pushing anything; a conflict fails in seconds and lists the files |
| `runner` | `planned` | for a single job suite, a runner label in place of the planned one (`full`, `merge`, `measure`: `xo-arm64-big`; the rest `xo-arm64`); `auto` always plans its own |

Runs never cancel each other: a concurrency group on REF and suite let an
agent's run cancel the lead's gate on the same ref, and every dispatch is
the same GitHub user, so it cannot be keyed per caller (removed
2026-10-14). Stop a duplicate by hand with `gh run cancel RUN_ID`. Inputs reach the shell through
environment variables, never pasted into the script. The checkout step is
`actions/checkout@v7` (Node 24; v4 printed a Node 20 deprecation warning).

The workflow file must exist on main for `gh workflow run` to find it;
`ref` then chooses which commit's tree is tested, and the workflow
definition comes from main (`gh workflow run ci.yml --ref BRANCH` takes
it from BRANCH instead, to try a change to ci.yml).

One run, several jobs (2026-10-14): the `plan` job (small runner, about
10 s) checks out REF, records REF's commit and main's, merges them, and
lists the jobs with `suite.sh plan`; the `test` jobs then run in
parallel, each on its own runner, each checking out the same two
commits, and the run passes only when every job does (`fail-fast` off,
so one failure does not stop the others). For `auto` the jobs are
`short` on `xo-arm64` and up to three on `xo-arm64-big`: the full tests
split by each package's weight (seconds on a big runner: internal/llvm
320, internal/selfhost 190, cmd/xo 100, gen and arm64 90, xotool 45,
others 30; longest first, each to the least loaded job) with the fixed
point (150) counted as one more load. A run watched with `gh run watch`
shows every job. Verified 2026-10-14 (run 38615252053, a probe
changing compiler/ and std/): four jobs in parallel (short 49 s;
selfhost, arm64, and std 3 min 19 s; cmd/xo, gen, and the fixed point
5 min 20 s; llvm 5 min 34 s), 6 min 18 s from the plan to the last job,
against about 8 minutes for the same tests in one job without cmd/xo.
`measure testworld` (run 38615416032): 3 min 27 s, ratios 1.000 for an
unchanged std.

## Suites

`scripts/ci/suite.sh SUITE [PACKAGE...]` runs the same on any machine
with Go and clang, so a failing suite can be reproduced locally.

| suite | runs |
|---|---|
| `auto` | `short`; then the full tests of the Go packages a changed file belongs to (its nearest directory holding `.go` files, so a `testdata` golden or `.xo` fixture tests its package), `./internal/selfhost` (merge tier) and `./cmd/xo` when anything under `compiler/` or `std/` changed, `./internal/gen`, `./internal/llvm`, and `./internal/arm64` (every backend's program output goldens) when `std/` changed, and `packages`; the fixed point when the change touches what the self build compiles (CLAUDE.md's list: `compiler/` but not `compiler/macho`, `compiler/wasm`, or `_test.xo` files; `std/native/`; `std/llrt/`; `internal/xotool` xotool.go, chain.go, seed.go, lock_*.go; `scripts/xobuild/`; `scripts/seed-chain.txt`). In the workflow these are parallel jobs (above); `suite.sh auto` on a machine runs them in order |
| `short` | `go vet` and `go test -short` over every package but `runs/` |
| `vet` | `go vet` only |
| `merge` | `go test -timeout 60m` of `packages` (default: the touched ones) |
| `fixed-point` | `go run ./scripts/xobuild` (the checkout's xo, from the newest seed) then `scripts/bootstrap-xo.sh` (s1, s2, s3; s2 and s3 identical) |
| `measure` | `bench/perf/<packages>/measure.sh` (one taking `<xo> <out dir> <backend...>`) with main's xo and std, then with the branch's, on arm64 and LLVM: both outputs (instructions:u, perf stat) and a `new/base` ratio per line. Linux numbers; decision 0180's bar stays the Mac's. It installs perf (`dnf install perf`) on a runner that lacks it, then fails with a clear message when perf is missing, `perf stat -e instructions:u` is refused (it prints kernel.perf_event_paranoid), or the measure.sh has no `perf stat` path. A darwin-only measure.sh (`/usr/bin/time -l` alone) otherwise fails with exit code 125 and no measurement line (runs 38656053811, 38656056910, 2026-10-14). text, json, and testworld count on Linux; atomic, digest, fsproc, gen, glob, httpclient, httpparse, portable, report, and unicode are still darwin-only (Last verified: 2026-10-14). When main's copy is darwin-only, the branch's measure.sh runs against main's workload sources A directory new on the branch (none on main) runs the branch's measure.sh for both, so main's xo and std build the branch's workload |
| `windows` | on the `xo-windows` runner: `scripts/windows-batch.sh` (xo1 built on the runner from the seed by scripts/xobuild with the Go backend, the LLVM goldens for windows/amd64 run and windows/arm64 cross built, after a preflight of one golden through the same test that stops the step with its error (2026-10-21), the LLVM runtime tests that run on Windows, `xo test` LLVM against Go). Best effort, not a gate (CLAUDE.md, arm64 only) |
| `script` | `packages` is `PATH [ARG...]`: runs PATH from the ref (`.sh` with bash, `.ps1` with Windows PowerShell 5.1, else as an executable) on `runner` (default xo-arm64, or `-f runner=xo-windows`). Environment: `XO_CLI` (the checkout's xo), `XO_CI_STD` (the checkout's std), `XO_CI_BASE_STD` (main's std, from a worktree of `XO_CI_BASE`), `XO_CI_OUT` (uploaded as the run's artifact `script-out`: `gh run download RUN_ID -n script-out`); native paths on Windows. For one-off comparisons like a bench/perf measure-windows.ps1 against main's std: `-f packages="bench/perf/sysio/windows-ci.sh [N]"` builds bench/perf/sysio with both stds and runs measure-windows.ps1 on each for two rounds (base, branch, base, branch; N runs per workload, default 15; WORKLOADS overrides the list) into `sysio-windows.txt` of the artifact (added 2026-10-14, not yet run). `bench/perf/http/burst-ci.sh [BLOCKS] [ROUNDS]` (2026-10-19, for one owner-approved run): the native Xo and Go HTTP servers on CPUs of their own, alternating blocks of `loadgen -burst 2000`, a summary in `burst.txt` (round wall p50/p99/max, rounds over 150 and 300 ms, latency p99, failures, minor faults; tested in Docker with stand-in builds). `scripts/ci/windows-race.sh` (2026-10-22, for one owner-approved run on `-f runner=xo-windows`): windows/amd64 --race with threads (decision 0273's Windows addendum): tsan_race, race_sync's 12 cases synchronized and not, and race_stacks, at XO_THREADS unset, 4 and 1, three rounds each (234 checks, `windows-race.txt` in the artifact); ThreadSanitizer's Go flavor from `ci/race/race_windows_amd64.syso` in the seed bucket, checked against the script's RT_SHA256 (scripts/race-runtime.sh windows-amd64 rebuilds it bit for bit); dry run under Wine with prebuilt binaries: 78 of 78 at REPS=1, not yet run on the runner. `scripts/ci/windows-crash.sh` (2026-10-22, decision 0343 stage 2, one owner-approved run on `-f runner=xo-windows`): the hardware fault report of windows/amd64 programs. The runner has no clang (run 39827577406: `use c` cannot bind a header), so the faults come from a scratch std module the script writes into a copy of std (std/native/crashpoke.xo: a write through address 16, Xo recursion without end, a thread started at a ud2) and builds with `--std --backend=amd64`: segv and a task's segv (the vectored handler), the thread's ud2 (the unhandled filter), the stack overflow (on_overflow's message, exit 2), XO_CRASH_REPORT=0; each checked for the report's lines and the NTSTATUS exit code (`windows-crash.txt` in the artifact). Dry run 2026-10-22: the build on the Mac, the five runs and the regexes under Wine (Box64), all passing. `scripts/ci/windows-paths.sh` (2026-10-23, one owner-approved run on `-f runner=xo-windows`): the host-path fixes of 629245c7e: `xo init` in a fresh directory, `xo test` twice with TMPDIR a `C:/` path (the second from the cache), `xo vet C:/...`, a contract-fuzz failure that writes regressions_test.xo, and a `--race` report with no doubled paths (`windows-paths.txt` in the artifact; XO_SKIP_RACE=1 skips the race step). Dry run 2026-10-23 on the Mac: 9 of 9 (race skipped), not yet run on the runner. Smoke tests: scripts/ci/script-env.sh and .ps1 (runs 38629095020, 38629098943, 2026-10-14) |
| `full` | every package with `XO_SELFHOST_TIER=full`, `-timeout 120m` (job limit 150 minutes); on demand only, before tagging or for a change the merge tier does not exercise |

"Touched" is `git diff --name-only $XO_CI_BASE...HEAD` (merge base with
`origin/main` by default), so with `merge_main` on it is still the
branch's own change. A Go package counts when a changed `.go` file's
directory still has `.go` files.

What stays on the Mac: Mac only checks (Mach-O, code signing, the darwin
fixed point), runs that write files back into a worktree (golden
recording; CI returns only logs), and the instruction counts the bar is
judged on (decision 0180: the Mac); `measure` adds Linux counts.

### Script output: text artifacts, large files in the bucket

Since 2026-10-22, the artifact `script-out` carries text only. That day
GitHub's artifact egress limit answered 503 on download, after scripts
uploaded their 43 MB input archive, unpacked trees and built binaries.

The scripts source `scripts/ci/ci-out.sh` (PowerShell:
`scripts/ci/ci-out.ps1`, `Send-CiOut`).

- **Inputs and build products** go to a work directory outside
  `XO_CI_OUT` (`ci_work NAME`, under `RUNNER_TEMP`), which is removed at
  exit. This covers archives, unpacked releases and programs the script
  builds.
- **Large outputs** move to the bucket at the end (`ci_offload DIR
  SUMMARY`): crash dumps and bundles, binaries kept to read the dumps
  with, and text over 1 MiB (`CI_OUT_MAX`). They go to
  `s3://$XO_SEED_BUCKET/dev/ci-out/$GITHUB_RUN_ID/<output dir>/`; the
  runner role may write `dev/*` (infra/ci-runners/iam.tf). Each one is a
  line in the script's summary: `large output in the bucket: KEY (N
  bytes)`. Fetch them with `aws s3 cp --recursive
  s3://$XO_SEED_BUCKET/dev/ci-out/RUN_ID/ DIR` on the Mac.

**Cleanup:** nothing expires under `dev/`. Delete a run's prefix once its
outputs are read: `aws s3 rm --recursive
s3://$XO_SEED_BUCKET/dev/ci-out/RUN_ID/`. `aws s3 ls
s3://$XO_SEED_BUCKET/dev/ci-out/` lists what is left.

Users: release-smoke.sh, windows-race.sh, windows-crash.sh, win-crash-cdb.ps1 and
win-cdb-watch.ps1. A new script that writes anything but small text
into `XO_CI_OUT` uses the same helper. Last verified: 2026-10-22 (on
the Mac with a local archive; not yet on a runner).

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
  (gotchas/lambda-reserved-concurrency-account-minimum.md). The webhook
  makes a start immediate (17 s from queued to running, measured); the
  sweep is the backstop for a lost delivery or a machine that was still
  stopping, and costs up to a minute (it alone gave the 75 s start of
  the first test, while the webhook was broken:
  gotchas/lambda-function-url-403-permissions.md).
- **Webhook security**: GitHub signs every delivery with HMAC-SHA256 of
  the body, keyed with the secret of this repository's webhook; the
  handler recomputes it with `/xo-ci/webhook-secret` and compares in
  constant time, so an unsigned or wrongly signed request gets 401
  (checked 2026-10-14). Only holders of the secret can sign: GitHub (for
  this repository's webhook only; other repositories, users, and apps
  cannot), and anyone who can read SSM `/xo-ci/webhook-secret` or the
  Terraform state (AWS account admins and the Lambda role). The handler
  does not act on the payload's contents: a signed delivery only means
  "look now", and it asks GitHub's API which jobs are really queued, so
  even a replayed or forged request can start a machine only for a real
  queued job (which needs repository write access to create), and that
  machine stops itself when idle. The function URL's resource policy
  allows the public `lambda:InvokeFunctionUrl` and
  `lambda:InvokeFunction` only with `lambda:InvokedViaFunctionUrl`, so
  the function cannot be invoked directly from outside the account
  (AWS provider 6.x). Rotate the secret with `terraform apply
  -replace=random_password.webhook` in infra/ci-runners (updates GitHub
  and SSM together).
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
  runs); toolchain (dnf git gcc make nodejs jq libicu; upstream LLVM 23.1.3 in
  /opt/llvm, pinned by SHA-256, whose clang links with lld through
  `clang.cfg` and is first on PATH as /usr/local/bin/clang, proved by
  `scripts/ci/lld-probe.sh`; Go 1.26.1 in /usr/local/go); the runner (latest release, in
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
| xo-ci-windows | i-EXAMPLE47 | c7i.4xlarge (16 vCPU, 32 GB), Windows Server 2025 | xo-windows | `suite=windows`, or any single job suite with `-f runner=xo-windows`; best effort |

All four were the lead's warm machines (scripts/warm-run.sh), adopted
into Terraform on 2026-10-14 with their 120 GB disks and caches. Because
the big machines also carry `xo-arm64`, a default job can run on one
when it is already up (GitHub picks any idle runner with the labels; the
autostart only decides which stopped machine to start).

The first boot showed a trap: restarting the runner service right after
`config.cmd --runasservice` started it made GitHub refuse the new
session ("Conflict: A session for this runner already exists") for a
few minutes; boot.ps1 now writes `.env` before config.cmd and restarts
only when `.env` changed on an existing registration.

Not runners (still plain warm machines, tag `purpose=xo-warm-run`):
three c7i.8xlarge amd64 (i-EXAMPLE02, i-EXAMPLE56,
i-EXAMPLE08).

The Windows runner (`"os": "windows"` in runners.json; runners.sh sets
it for `adopt` of a Windows instance and for `add` with a label starting
`xo-windows`) boots from `boot/windows.ps1`, the counterpart of
linux.sh: user data `<powershell>` with `<persist>true</persist>` runs
on every boot (EC2Launch v2) and fetches the script from
`s3://xo-seeds-ACCOUNT_ID/ci/boot/windows.ps1`. It keeps the 8 hour
cap (scheduled task `xo-warm-cap`), the idle stop (scheduled task
`xo-ci-idle` every minute, `C:\ProgramData\xo-ci\idle.ps1`; the lifetime
cap's pending shutdown is cancelled first, since Windows refuses a
second one), Defender exclusions for the runner and build trees
(gotchas/windows-defender-slows-builds.md), the toolchain of
scripts/warm-run.sh's Windows user data (Git for Windows, Go, LLVM, the
MSVC build tools, the AWS CLI; skipped when `C:\xo-ready` exists), and
the runner as a service running as SYSTEM in `C:\actions-runner`. Its
`.env` puts Git Bash first on PATH (the test job's steps run with
`shell: bash`) and points HOME, USERPROFILE, LOCALAPPDATA, and GOPATH at
Administrator's profile, so the warm machine's Go and xo caches stay in
use. Log: `C:\ProgramData\xo-ci\boot.log`. After adoption
scripts/warm-run.sh no longer finds it (`Name=xo-warm`) and would create
a new Windows instance: use CI instead.

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
| Lambdas | `xo-ci-register`, `xo-ci-autostart` | Node.js 22; code `infra/ci-runners/lambda/`; autostart 256 MB (128 MB ran at 121 MB); logs `/aws/lambda/<name>`, 14 days |
| function URL | xo-ci-autostart | auth NONE; resource policy: public InvokeFunctionUrl and InvokeFunction only via the URL; the handler checks GitHub's `X-Hub-Signature-256` |
| schedule | EventBridge rule `xo-ci-autostart-sweep` | `rate(1 minute)`, input `{"sweep": true}` |
| boot scripts | `s3://xo-seeds-ACCOUNT_ID/ci/boot/linux.sh`, `.../windows.ps1` | rendered from boot/linux.sh and boot/windows.ps1 |
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
  `gh api repos/josephspurrier/xo/hooks/697891712/deliveries`): every
  delivery should be 200. A 403 means the function URL's permissions
  (gotchas/lambda-function-url-403-permissions.md), a 401 a wrong secret;
  redeliver one with `gh api -X POST .../deliveries/ID/attempts`. A
  missed delivery only delays a start until the next sweep.
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

From the first runs, 2026-10-14 (runs 38595689932, 38596682112,
38597216716, 38597341138, 38599119717, 38600224256):

| what | time |
|---|---|
| `short` (vet and `go test -short`, every package) on xo-ci-arm64 | 51 s step, 81 s queued to done |
| stopped machine to job running, through the webhook (delivery 5 s, Lambda starts xo-ci-arm64-big2, runner online and job assigned 12 s later) | 17 s |
| the same through the one minute sweep only (webhook broken at the time) | 75 s |
| idle stop after the last job | 5 min 24 s |
| `auto` on a branch changing compiler/lexer/dump.xo and a file under internal/gen/testdata (short, internal/gen 80 s, internal/selfhost 161 s, fixed point) | 6.5 min |
| xo-ci-windows first boot after adoption (boot.ps1 start to registered; the toolchain was already there) | 14 s |
| `windows` (run 38627432745): plan with the xo.exe cross build 18 s, then the Windows job (checkout, windows-batch.sh steps 1 to 4) | 5 min 33 s job; 156 of 157 windows/amd64 goldens pass (proc_cmd fails, below), 157 of 157 windows/arm64 built, LLVM runtime tests ok, `xo test` LLVM against Go 16 same, 0 differ |
| `windows` without the cross build (run 38633141732): plan 4 s, the Windows job with xo built on the runner (scripts/xobuild, seed cached) | 6 min 43 s job; 157 of 157 goldens pass, 157 arm64 built, runtime ok, 16 same |
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
- Webhook plus a one minute sweep over polling alone: the webhook starts
  a machine at once (17 s to a running job), and a lost delivery costs at
  most a minute.
- The machines stop themselves: no outside system can stop a machine in
  the middle of a job, and nothing depends on the Mac.
- It replaces the lead's Mac-side EC2 queue (ec2q-runner4, warm-run2,
  stop-idle-loop2): retired 2026-10-14. Never run them against the
  `xo-ci-*` machines; they look for `Name=xo-warm` and would create new
  instances.

## Not done yet

1. Time the full tier on CI: `gh workflow run ci.yml -f suite=full -f
   runner=xo-arm64-big`.
2. amd64 runners (best effort): `runners.sh adopt` the three c7i.8xlarge
   machines with label `xo-amd64`, add the choice to `ci.yml`'s `runner`
   input; the fixed point there uses the LLVM backend.
3. Windows known issues (best effort, recorded, not gates; Last
   verified: 2026-10-14, run 38627432745):
   - the golden `proc_cmd` failed under the runner (fixed 2026-10-14,
     run 38629617435 showed the diff): it printed the child shell's
     `$HOME` with HOME left out of the environment, which Git Bash's sh
     fills from the account's profile, so want_windows.txt held the
     Administrator path and the SYSTEM service printed
     `/c/Windows/system32/config/systemprofile`. The echo no longer
     prints HOME.
   - XO1306 on Windows (gotchas/windows-seed-xo1306-module-not-required.md):
     fixed 2026-10-14 in internal/xotool (/ paths to xo); a Windows
     runner builds xo from the 0.11.0 seed in 56 s
     (scripts/ci/xobuild.sh, run 38629916521), so the plan job's cross
     build of xo.exe (2026-10-14 runs up to 38632543739) is gone.
     Still open: the driver accepting `C:\` arguments itself.
   - `suite=windows` passes in full: run 38630848188.
4. Decide whether four big machines are needed; each stopped one costs
   about $9.60 a month.
