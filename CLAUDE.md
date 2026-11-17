# Xo

Xo is a small, statically typed language for backends and web services,
designed for AI agents to write, check, cross compile, and debug. The
compiler and the `xo` toolchain are written in Xo (compiler/); the Go
implementation the language started with was deleted (decision 0215).

Agents start each task in a fresh session (owner, 2026-11-10), so this
file is the whole briefing: read it, then your resume note, then
`knowledgebase/infrastructure/agent-tools.md`. The dated record of how
the test and push rules grew is
[test-policy.md](knowledgebase/infrastructure/test-policy.md); where it
differs from this file, this file is current.

## Safety rules for every agent (required)

- Never run `pkill`, `killall`, or `kill` with a name or pattern. `pkill -f`
  matches anywhere in a command line, so `pkill -f cat` killed every app under
  `/Applications` on 2026-10-07. Only `kill <PID>` for a process this session
  started and whose PID it captured itself (never a PID found by `ps | grep`).
- Never stop, signal, or restart processes, services, VMs, or apps the
  session did not start, on the Mac or on any host. Hung command: let its
  timeout expire or stop the background task through the harness. heavyd is
  restarted only by the owner.
- Subagent prompts must repeat these rules.
- Secrets and private values (accounts, buckets, IPs, passwords, tokens)
  never go into tracked files, commit messages, or the website; they live
  in untracked `CLAUDE.local.md` and `~/xo-lead/*.env`. Never print them.
- The owner decides publishing (releases, public repos, registries, PRs to
  outside projects), system installs, IAM, and seed versions. Read an
  outside project's contribution and AI policy before preparing anything
  for it.
- The OCI host (module proxy, checksum log) is hosting only: never a test,
  gate, or benchmark host.

## Each task, start to finish

1. Start: read this file, your resume note (the path the lead gave you),
   and agent-tools.md; `XW_AS=NAME ~/xo-lead/xw/bin/xw queue NAME` for
   your queue (NAME: xo-lang-1, xo-lang-2, xo-lang-3, azure-1, xw-1; ADR
   0658). Ack each assigned task and each decision sent to you in the
   same tool round (`xw task ack ID`, `xw ack ID`; `xw decisions --open
   --agent NAME`). Your top task is the next item; a task marked
   OFF-CHARTER needs the lead's OK first.
   Check `knowledgebase/decisions/` and
   [xo-at-a-glance.md](knowledgebase/infrastructure/xo-at-a-glance.md)
   before proposing or reporting a feature as missing.
2. Work in a worktree under `~/xo-wt/` (never in the main checkout, never
   under /tmp, which a reboot wipes): `git worktree add -b NAME
   ~/xo-wt/NAME origin/main`. A new worktree before the next item's first
   edit, since a running gate reads its tree. Never `git stash` (shared
   across worktrees); use `git pull --rebase --autostash`. Scripted edits
   use absolute paths and assert one match. Never edit a script while it
   runs (bash reads it as it goes); copy it to a new name.
3. Find code with `xo def NAME`, `xo refs NAME`, `xo type FILE:LINE:COL`,
   `xo doc NAME`, `xo symbols [--all]` (`--dir=DIR`, `--json`; exit 1 no
   match, 2 usage error) before grep (decision 0261); say in your report
   when one falls short. Check compiler edits with
   `go run ./scripts/xocheck DIR`.
4. Run every build, test, benchmark, or repro through heavy.sh, even a
   one-off: `HEAVY_CLASS=gate ~/xo-lead/agents/heavy.sh NAME CMD...` for a
   gate or anything under about 2 minutes. Long pass/fail runs go to an
   offload host (below); `HEAVY_CLASS=bulk` on the Mac only with
   `HEAVY_DARWIN=1` for darwin-only work. Logs go to a file outside the
   worktree; read `tail -20` or a `grep -E 'FAIL|panic|error'`, never a
   whole log. Use `set -o pipefail` before a pipe whose status matters.
5. Gate: `pushq.sh WORKTREE --needs` lists the checks the push will ask
   for; run them with `OFFGATE_AGENT=NAME ~/xo-lead/agents/offload-gate.sh
   WORKTREE [STEP...]` (scripts/gate.sh on the pass/fail host; the steps
   are in agent-tools.md) plus the one or two tests that prove the change.
   Per push only the bare minimum (below).
6. Push only through `~/xo-lead/agents/pushq.sh WORKTREE --gated` with
   `PUSHQ_RAN` as the gate printed it (`--docs` for documentation only).
   Squash onto your own merge base (`git reset --soft "$(git merge-base
   HEAD origin/main)"`), never onto a moved origin/main. No attribution
   lines in commits.
7. Update the knowledgebase in the same change (below), then report and
   hand over (token budget, below). The tools mark the task (#246): name
   it once with `XW_TASK=ID` on the first heavy.sh or offload-gate.sh run
   in its worktree (start, then gate), `PUSHQ_TASK=ID` or the binding
   marks the push, and `pushq.sh WT --gated --done ID` the last push
   (push, then done). You call only `xw task ack ID` and `xw task block
   ID REASON` (a block also goes to the lead); `xw task done ID` for a
   task that ends without a push.

Never wait idle: while a gate, push, or long run goes, work the next item
in another worktree. Only an empty queue stops you; then ask the lead.

## Token budget and handover (owner, 2026-11-09 and 2026-11-10)

- Reports to the lead are at most 5 lines (result, block, or decision),
  with details in a file under `~/xo-lead/notes/AGENT/` named in the
  report. Message the lead only for results, blocks, decisions, and
  approvals; status is your tasks' states in xw (heavy.sh, pushq and
  offload-gate record jobs, gates and pushes themselves).
- No polling (owner, 2026-11-15: polls were about 30% of agent calls,
  each re-reading the whole context): wait with `heavy.sh --wait NAME`,
  `pushq.sh --wait WT` or `offload-gate.sh --wait WT` under
  run_in_background and act on the notification (agent-tools.md
  "Waiting"); never a sleep loop or repeated tail or status checks;
  outside CI and registries at most once a day.
- After each finished task: update your resume note (open branches and
  worktrees, queued gates and jobs, next item, key paths), mark the task
  in xw, and go straight to your next item. No restart after a task
  (owner, 2026-11-17, xw decision 55: restarts and compactions cost more
  of the plan than large contexts; auto-compact is "auto"); a session is
  restarted only when it misbehaves or the owner asks.

## Everything goes through xw (owner, 2026-11-17)

xw (ADR 0658) is the system of record; nothing that xw can hold lives
only in a note, a chat or a message:

- Decisions: the lead records each owner decision with `xw decide --to
  AGENTS` (agents ack with `xw ack ID`); owner questions go through
  `xw ask` and `xw answer`, never only in chat or SendMessage.
- Directives and work: `xw task add` and `xw decide`, which the xw hook
  shows the agent; no duplicate SendMessage (only for something urgent
  or an idle agent).
- Measures: usage, calls and job time come from `xw activity`, `xw jobs`
  and `xw gates`; a measure xw lacks becomes an xw-1 task, not a
  private script. Recurring checks become xw alerts (cost review, #252).
- Remove, don't tune (xw decision 58): a step, script or rule costing
  more than a few minutes is first asked whether Xo or xw can remove it.

## How we work (owner rules, 2026-10-09 to 2026-11-16)

- No forks and no helper subagents, for the lead or any worker (owner,
  2026-11-15: "a helper agent is another agent"). The lead assigns work
  to a worker session; each worker does its own work in its session.
  Agents talk by SendMessage, never by typing into another session.
- Ports and gap lists: port every piece first with Go's unit tests, push
  in batches (one gate per 3 to 5 pieces), then one combined evidence
  pass (interop, fuzz, perf `--ab`, stress) at the end, never a full
  evidence cycle per piece (owner, 2026-11-15).
- Stability and test-gap fixes need no go-ahead: do them and report
  (owner, 2026-11-11). Ask only for publishes, releases, tags, and
  design choices.
- Test on arm64 (the Mac, linux/arm64 hosts). An x86 or Windows-only
  bug gets one targeted job on that arch with a cross-built xo, never a
  full tier there (owner, 2026-10-14).
- Xo stays on 0.x minors; only the owner declares 1.0.
- Every claim to the owner or the lead names its source (commit,
  decision file, log line) or says unverified. Hashes, run IDs, figures
  and ratios are copied from command output run in an earlier step,
  never typed from memory. Numbers for the website come only from a
  committed results file, in its own wording.
- Before reporting that an xo tool missed something, rerun it and count
  its whole output (`| wc -l`), not a `head` slice.
- Comparisons with Go are like for like: both tools with their default
  caches; cache-off rows are a labeled diagnostic, never the headline.
- Commands written for the owner (OWNER-*.md kits, notes) are one line
  each, with no `\` continuations (`grep -c '\\$' FILE` gives 0).

## Agents (as of 2026-11-10)

| Session | Area |
|---|---|
| Xo Lang 1 | compiler, backends (arm64, x86-64, LLVM), new targets (FreeBSD, riscv64), the language |
| Xo Lang 2 | toolchain, releases, Windows (native windows/arm64), website, browser playground |
| Xo Lang 3 | std (ports from Go's standard library, std/changes logs) |
| Azure 1 (tab "Xo AWS") | cloud SDKs, editors and LSP, pkg.xolang.org |
| xw-1 | workstream (xw), the agent work system (decision 0658; private repo xo-lang/workstream) |
| Xo Lead | merges, tiers, hosts, owner decisions |

## Where things run

- The Mac Studio (30 cores, 96 GB): gates (start when 35% of memory is
  free, fewer than 5 gates run and the load is under 48), darwin-only work,
  4 heavy slots, each job stopped past 32 GB. `heavy.sh --status` shows
  every lane. Last verified: 2026-11-10.
- Offload hosts ([offload.md](knowledgebase/infrastructure/offload.md)):
  host 2 (`offload-arm64b.env`) and host 3 (`offload-arm64c.env`) for
  linux/arm64 pass/fail (`offload-gate.sh`, `offload.sh`); host 1
  (`offload.env`) for timing only (`ab-host.sh` for `--ab`, exclusive
  runs); linux/amd64 (`amd64-run.sh REV NAME sample`); Windows amd64
  (`OFFLOAD_ENV=offload-win.env`); the windows/arm64 VM
  (`offload-winarm.env`, Xo Lang 2's lane). Any test, build, or
  measurement may run there without asking. A failure seen on one host
  only is observed on that host first.
- CI is retired (decision 0423); `gh workflow run ci.yml` has no runner.

## Tests and pushes (current rules)

Run the minimum and fix forward (owner, 2026-10-09 to 2026-11-06; the
reasons are in test-policy.md):

- Std Proven push (owner, 2026-11-15: "I'm fine with a couple bugs after a
  4 hour work session that takes a few minutes to fix but saved hours in
  running the same tests"). Until std is Proven this overrides the rules
  below for std and compiler work:
  - a push changing only std modules outside xo's closure (what
    compiler/xo builds from, `xo build --list-inputs`) gates on the
    module's own tests (`xo test --std-module`), stdchecks and stdgen
    -check: minutes, no golden suites;
  - a push changing xo's closure (compiler/, std/native, llrt, or a
    closure std module) gates on selfbuild plus seedbuild and the one or
    two tests that prove the change; no arm64, llvm or cmd suites;
  - the full suites run on main every 2 hours (the lead's tier on the
    offload hosts); a failure there is fixed forward by the commit's owner
    within the hour, never reverted unless xo cannot build itself.
- Per push: the gate (one xo build with the change plus the one or two
  tests that show it works), with these additions when they apply:
  - `codegen` for compiler/ssa, compiler/arm64, compiler/llvm changes;
    `wasm` (targeted `TestSelfhostWasm`) for std/native/llrt or
    std/native/rt*.xo; pushq refuses without them;
  - `selfbuild` and `seedbuild` when the change touches what compiler/xo
    builds from (compiler/ except compiler/macho, compiler/wasm and
    `_test.xo` files; std/native; std/llrt);
  - the amd64 sample on the linux/amd64 host for x86 code generation or a
    change to the C compiler or linker flags xo passes (that host's cc is
    gcc);
  - `xo dev codegen --backend=arm64,llvm test/codegen` for crypto and
    codegen changes; `TestCodegenInputsClassified` for a new `XO_*`
    variable or a compiler file that reads the file system;
  - a new `rt.*` intrinsic is lowered in the wasm runtime too, with the
    wasm goldens run;
  - a changed default (an `XO_*` or flag default, a default-on pass)
    runs the full suites of the packages it touches; a runtime limit or
    allocation change that xo itself runs gates on a self build with the
    new xo (`scripts/bootstrap-xo.sh s2`);
  - no identity check unless the push claims no output change
    (`scripts/identity BASE NEW`), no benchmark sweep unless it claims a
    speedup.
- Speed claims carry an `xo bench --ab` result (decision 0340), run on
  host 1 unless darwin specific; callgrind and `XO_PROF` explain where
  time goes but never prove a speedup. A codegen-only change reports
  callgrind's instruction count and confirms with one `--ab`.
  Scheduler, lock, spin, I/O, and concurrency changes use `--ab` only.
- The lead runs `go test -short` plus the full fixed point on main every
  2 hours and the full tier every 8 hours, and bisects a failure to the
  commit's owner, who fixes forward with a regression test. A failure on
  main that is not yours: message the lead with the error and the good
  and bad commits; the lead names one bisect owner.
- An old xo (a bisect step, an identity base) runs with a private
  `XO_BACKEND_CACHE_DIR` deleted afterwards. Never test a newer std with
  an older bin/xo; rebuild after a rebase.
- No rerun after a rebase unless the incoming commits change what the
  change touches or depends on, or a conflict changed code.
- Every bug fix, however it was found (a failure on main, an agent's own
  test, an audit, a profile), lands with a test that fails before the fix
  and passes after it (owner, 2026-11-15). A speed regression gets a
  guard instead: an instruction-count or time-budget test on the case
  that regressed. Mark the xw task as a bug fix; xw flags a fix pushed
  without a test file.
- Golden tests: `want.txt` uses the exit/stdout/stderr layout
  (`xo dev new-golden`); run goldens through their tests (arm64
  `TestGoldenRun`, llvm `TestGolden`), and check the run matched tests
  (a wrong `-run` passes as "no tests to run"). A cold LLVM golden run
  takes over 5 minutes: give `go test` a `-timeout` above its 10-minute
  default when other packages share the run.

## Where things live

- [`knowledgebase/infrastructure/agent-tools.md`](knowledgebase/infrastructure/agent-tools.md):
  every agent tool (lookups, miscompile triage, std tests, measuring,
  push queue and board) and when to use it.
- `spec/core.md`: the language specification, the source of truth for
  syntax and semantics. `spec/std.md`: prelude methods and the standard
  library surface.
- `knowledgebase/infrastructure/`: toolchain, repo layout, build and test
  commands, hosts, namespaces (GitHub org, Homebrew, etc.).
- `knowledgebase/decisions/`: design decisions as numbered ADRs
  (`NNNN-short-title.md`).
- `knowledgebase/gotchas/`: traps and surprises, with how to avoid them.
- `bench/`: benchmark tasks and results that decide the `Proposed`
  decisions. `bench/paper/` holds solutions written blind from the spec;
  do not edit them.
- xw (ADR 0658): `~/xo-lead/xw/bin/xw status|queue|priority|check`,
  agent-tools.md "Status and work queue".
- Profiling native programs: `XO_PROF=cpu` / `xo run --profile=cpu`
  (decision 0238, toolchain.md Profiling).

Read `knowledgebase/README.md` and the relevant folder index before
starting work in an area.

## Keeping the knowledgebase current (required)

Update the knowledgebase in the same change as the work, not later:

- **Spec**: any change to syntax or semantics updates `spec/core.md`. If code and
  spec disagree, fix one of them; never leave them inconsistent.
- **Decisions**: when a design or architecture choice is made (by the owner or
  agreed in conversation), add `knowledgebase/decisions/NNNN-title.md` with
  Status, Context, Decision, Consequences, Alternatives. To reverse a decision,
  add a new ADR and mark the old one `Superseded by NNNN`; never delete.
  Answering one of the spec's Appendix B open questions requires an ADR.
  Claim the number first with `~/xo-lead/agents/adr-claim.sh SLUG WHO`
  (diagnostic codes: `code-claim.sh PREFIX WHO`), then commit a stub
  `NNNN-title.md` (Status: Proposed) and its README line on main, then
  fill it in. `TestDecisionNumbersUnique` fails on a duplicate number.
- **Std change logs** (decision 0276): a user visible change to a std
  module (fix, port, behavior, security, feature, perf) adds an entry to
  `std/changes/MODULE.md` in the same commit, in 0276's format. Std
  follows Go's standard library (port Go's code and tests where they
  fit, with attribution; Xo's own API). Port Go's fast paths too, and
  give every port a timing row against Go's package beside its corpus
  (gotchas/std-port-costly-go-idioms.md: small Lists allocate, no
  package-level caches; Str/Bytes slices are views since decision 0330).
- **CHANGELOG**: a user visible change adds its line to CHANGELOG.md
  (Breaking changes say what users do).
- **Gotchas**: when something breaks unexpectedly, behaves surprisingly, or
  costs more than one attempt to get right, add a gotcha with symptom, cause,
  and fix. Mark it `Observed` or `Anticipated`; flip anticipated entries to
  observed (or delete them if they turn out wrong) once hit.
- **Infrastructure**: when commands, layout, toolchain versions, dependencies,
  or hosts change, update `knowledgebase/infrastructure/`. Put a
  `Last verified: YYYY-MM-DD` line on facts that can go stale.
- Every new file gets a one line entry in its folder's `README.md`.
- Use absolute dates (2026-10-07), not relative ones. Never use em-dashes.
- Before finishing a task, check: did anything in this task change the spec,
  make a decision, reveal a gotcha, or change infrastructure? If yes, update
  the knowledgebase before reporting done.

## Conventions and facts

- Write the language name as **Xo** in prose; lowercase `xo` only for the
  CLI, the `.xo` extension, `xo.mod`/`xo.lock`, and the `xo-lang` namespace.
- Go code that remains (test drivers, scripts, std/rt): standard library
  preferred, `gofmt`, table driven and golden file tests under `testdata/`.
- The Go compiler is gone (decision 0215; last commit tagged
  `go-compiler-final`, 87873aa5): `xo` is built from compiler/xo, and
  compiler/, std/ and the spec are the source of truth. Every compiler
  change lands in compiler/. Still in Go: std/rt (the Go backend's
  runtime), internal/gobind and cmd/xo-gobind (`use go`),
  internal/gotool, the test infrastructure (internal/xotool, progen,
  testtmp, mod/modtest), and test-only packages (cmd/xo, internal/llvm,
  gen, arm64, selfhost, repotest) that drive the xo binary.
- The xo the Go tests and scripts run: `$XO_CLI`, else the checkout's
  `bin/xo`, which `go run ./scripts/xobuild` (and any Go test that needs
  it) rebuilds when compiler/ or a std module it uses changed, cached per
  source key in `<user cache>/xo/checkout/<key>`. Never xobuild a dirty
  tree with ABI changes into the shared cache. Set `XO_CLI` to a built xo
  to skip rebuilds while iterating.
- Seeds (decision 0216): release N builds with release N-1. Prebuilt
  seeds live in the private S3 bucket named by `$XO_SEED_BUCKET` (values
  in the untracked CLAUDE.local.md; decision 0249). Seeds are internal,
  never published. When a change lands that older seeds cannot build,
  the minor version is bumped and a seed cut (`scripts/seed-cut.sh`, then
  `scripts/seed-upload.sh`; toolchain.md "Cutting a seed"); the lead cuts
  seeds. If a seed cannot be fetched, stop and report: never fall back to
  the go-compiler-final chain (25 to 37 minutes).
- The TestSelfhost* dump replays are skipped by default (decision 0217);
  `XO_SELFHOST_DUMPS=all` runs them for a deliberate audit. The compiler
  is verified by behavior (output goldens on every backend, the fixed
  point, error and codegen pattern tests, progen, xo unit tests). Never
  re-record dumps to get a change merged.
- internal/selfhost tiers (`XO_SELFHOST_TIER`): `short`, `merge` (the
  default), `full` (every backend, on an offload host through the lead).
- Diagnostics are JSON with stable `XOnnnn` codes (spec section 10). Never
  change the meaning of an existing code; add a new one.
- Generated Go must carry `//line` directives back to `.xo` sources.
