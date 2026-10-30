# infra/ci-runners: self-hosted GitHub Actions runners on EC2

**Retired 2026-10-29 (decision 0423):** the machines are terminated,
runners.json is empty, the GitHub runners are removed, and the stack is
destroyed (`terraform destroy` here). `terraform apply` with runners.json
entries would rebuild it.

The on demand CI of decision 0225: the warm EC2 machines run GitHub
Actions jobs for `josephspurrier/xo`, start when a job is queued, and
stop themselves when idle. The full reference (running CI, how it
works, inventory, operations, cost) is `knowledgebase/infrastructure/ci.md`;
this file covers the Terraform pieces.

## Pieces

- `runners.json`: the runners, `NAME -> {type, arch, labels, az,
  import_id}`. The GitHub runner is `xo-ci-NAME`. Edit it with
  `runners.sh`.
- `runners.tf`: one EC2 machine per entry (AL2023, 120 GB gp3, shutdown
  behavior stop, tags `purpose=xo-ci`, `ci-runner`, `ci-labels`). An
  entry with `import_id` adopts an existing machine; its AMI and subnet
  are ignored so it is never replaced.
- `boot/linux.sh`: rendered to `s3://xo-seeds-ACCOUNT_ID/ci/boot/linux.sh`;
  user data runs it on every boot. It sets the 8 hour cap, the idle stop
  (power off after `idle_minutes`, default 5, without a job), the
  toolchain, and the runner (registered again whenever GitHub has
  forgotten it). Log: `/var/log/xo-ci-boot.log` on the machine.
- `lambda/`: `xo-ci-register` hands a machine a registration token (the
  GitHub App key stays in SSM, readable only by the Lambdas);
  `xo-ci-autostart` receives the `workflow_job` webhook (HMAC checked) and
  runs every minute, starting a stopped machine whose labels cover a
  queued job, smallest label set first.
- `webhook.tf`: the repository webhook and its secret
  (`/xo-ci/webhook-secret`).
- `iam.tf`: role `xo-ci-runner` (seeds read, `dev/*` write, invoke
  xo-ci-register) and role `xo-ci-lambda` (SSM `/xo-ci/*`, describe and
  start `purpose=xo-ci` instances).
- `github-app/create-app.py`: made the `xo-lang-ci` GitHub App (id
  5318159, installation 171424164, on josephspurrier/xo only;
  permissions administration write, actions read, webhooks write,
  metadata read) and stored it in SSM `/xo-ci/github-app/*`. Run it again
  only to make a new App.
- State: `s3://xo-tfstate-ACCOUNT_ID/ci-runners/terraform.tfstate`
  (infra/state).

## Runners

```
infra/ci-runners/runners.sh list                          # machine state, GitHub status, labels
infra/ci-runners/runners.sh add arm64-2 r8g.4xlarge xo-arm64
infra/ci-runners/runners.sh add arm64-big r8g.8xlarge xo-arm64-big,xo-arm64
infra/ci-runners/runners.sh adopt arm64-big i-EXAMPLE01 xo-arm64-big,xo-arm64
infra/ci-runners/runners.sh remove arm64-2                # deregisters, deletes machine and disk
infra/ci-runners/runners.sh start|stop NAME
```

`add`, `adopt`, and `remove` show the Terraform plan and ask first
(`XO_TF_AUTO=1` skips the question). A big machine also carries the
`xo-arm64` label, so default jobs overflow onto it when the small one is
busy; the autostart starts the small one first.

Debugging a machine: `ssh -i ~/.ssh/xo-ci.pem ec2-user@IP` (security
group xo-ci-ssh), `sudo tail /var/log/xo-ci-boot.log`, `journalctl -u
xo-ci-idle`. Lambda logs: CloudWatch `/aws/lambda/xo-ci-autostart`.

Last verified: 2026-10-14 (runner xo-ci-arm64 registered and ran
`ci short main`).
