# CI: on demand GitHub Actions on EC2

Decision 0225. Nothing runs on push, pull request, or a schedule: every
run is started by hand, by an agent, or by the owner. The machines are
self-hosted runners on EC2 (infra/ci-runners): a queued job starts a
stopped machine within about a minute; machines stop after 5 idle
minutes. Last verified: 2026-10-14.

## Running CI

```
gh workflow run ci.yml -f ref=BRANCH -f suite=auto       # merged with main by default
gh run list --workflow ci.yml --limit 5                  # run names: "ci SUITE REF [+main]"
gh run watch RUN_ID --exit-status                        # blocks until done; exit status is the run's
gh run view RUN_ID --log-failed                          # only the failing step's log
```

Inputs (`.github/workflows/ci.yml`):

- `ref`: branch, tag, or commit (default main). Push the branch first.
- `suite` (`scripts/ci/suite.sh`, which runs the same anywhere):
  - `auto`: `short`, then the full tests of the Go packages the change
    touched, the internal/selfhost merge tier when `selfhost/` or `std/`
    changed, and the fixed point (`scripts/bootstrap-xo.sh`, s2 equal to
    s3) when the change touches what the self build compiles (CLAUDE.md's
    list). "Touched" is the diff from the merge base with main.
  - `short`: `go vet` and `go test -short` over every package but runs/.
  - `vet`, `merge` (full tests of `packages`, default the touched ones),
    `fixed-point`, `full` (every package, `XO_SELFHOST_TIER=full`, 150
    minute limit; on demand only, use `runner=xo-arm64-big`).
- `packages`: for `merge`, space separated (`./cmd/xo ./internal/llvm`).
- `merge_main` (default true): test REF merged with the current main; a
  conflict fails in seconds and lists the files.
- `runner`: `xo-arm64` (default) or `xo-arm64-big`.

Starting the same REF and suite again cancels the earlier run.

## Where it runs

linux/arm64 only (decision on arm64 only testing, 2026-10-14). Mac only
checks (Mach-O, signing, the darwin fixed point) and anything that writes
files back into a worktree (golden recording) stay on the Mac. Machines,
labels, adding and removing runners: infra/ci-runners/README.md.

Cost: the machines' on demand hours while running (r8g.4xlarge about
$0.94 an hour, r8g.8xlarge about $1.89) plus their stopped disks (120 GB
gp3 each, about $9.60 a month); the Lambdas and GitHub's side cost
nothing (GitHub postponed its self-hosted runner fee in December 2025).
