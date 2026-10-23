# Xo

Xo is a small, statically typed language for backends and web services,
designed for AI agents to write, check, cross compile, and debug. The
compiler and the `xo` toolchain are written in Xo (compiler/); the Go
implementation the language started with was deleted (decision 0215).

## Safety rules for every agent (required)

- Never run `pkill`, `killall`, or `kill` with a name or pattern. `pkill -f`
  matches anywhere in a command line, so `pkill -f cat` killed every app under
  `/Applications` on 2026-10-07. Only `kill <PID>` for a process this session
  started and whose PID it captured itself.
- Never stop, signal, or restart processes, services, or apps the session did
  not start. Hung command: let its timeout expire or stop the background task
  through the harness.
- Subagent prompts must repeat these rules.

## Where things live

- [`knowledgebase/infrastructure/agent-tools.md`](knowledgebase/infrastructure/agent-tools.md):
  every agent tool (lookups, miscompile triage, std tests, measuring,
  push queue and board) and when to use it; read it at the start of a
  session.
- [`knowledgebase/infrastructure/xo-at-a-glance.md`](knowledgebase/infrastructure/xo-at-a-glance.md):
  what Xo already has (types, statements, concurrency, std, tools) with
  spec sections; check it before reporting a feature as missing (owner,
  2026-10-21: match guards and Set were reported missing).
- Finding Xo code (decision 0261): use `xo def NAME` (`Type.method` or a
  bare name), `xo refs NAME`, `xo type FILE:LINE:COL`, `xo doc NAME`
  (a type's methods), and `xo symbols [--all]`, with `--dir=DIR` and
  `--json`, instead of grep for declarations, references, types, and
  available methods; exit 1 means no match, 2 a usage error.
  Try them before grep every time (owner, 2026-10-21: half the agents
  still reached for grep by habit); when one falls short, say so in your
  report so the tool gets fixed.

- `spec/core.md`: the language specification. Source of truth for syntax and
  semantics.
- `spec/std.md`: prelude methods and standard library surface.
- `knowledgebase/infrastructure/`: toolchain, repo layout, build and test
  commands, CI, namespaces (GitHub org, Homebrew, etc.).
- [`knowledgebase/infrastructure/ci.md`](knowledgebase/infrastructure/ci.md):
  on demand CI (decision 0225): how to run it, the suites, the
  self-hosted EC2 runners (`infra/ci-runners`), the AWS and GitHub App
  inventory, operations, measurements, and cost.
- `knowledgebase/decisions/`: design decisions as numbered ADRs
  (`NNNN-short-title.md`).
- `knowledgebase/gotchas/`: traps and surprises, with how to avoid them.
- `bench/`: benchmark tasks and results that decide the `Proposed` decisions.
  `bench/paper/` holds solutions written blind from the spec; do not edit them.
- Profiling native programs: `XO_PROF=cpu` / `xo run --profile=cpu`
  (decision 0238), see toolchain.md Profiling; use it before Docker
  callgrind for where the time goes. (Owner-requested, 2026-10-16.)
- Measuring speed (owner, 2026-10-22): `xo bench --ab` (decision 0340)
  decides whether a change is faster; callgrind and `XO_PROF` explain
  where the time goes and never prove a speedup alone. Every speedup
  claim carries an `--ab` result. Scheduler, lock, spin, I/O and
  concurrency changes use `--ab` only: callgrind runs threads one at a
  time, so spinning looks free and fiber switches are charged to the
  wrong function (the spin default and the h2 "allocator slow path").
  A codegen only change reports callgrind's instruction count and
  confirms it with one `--ab` run. See agent-tools.md "Measuring".

Read `knowledgebase/README.md` and the relevant folder index before starting
work in an area. Check `decisions/` before proposing a change to something
already decided.

## Keeping the knowledgebase current (required)

Update the knowledgebase in the same change as the work, not later:

- **Spec**: any change to syntax or semantics updates `spec/core.md`. If code and
  spec disagree, fix one of them; never leave them inconsistent.
- **Decisions**: when a design or architecture choice is made (by the user or
  agreed in conversation), add `knowledgebase/decisions/NNNN-title.md` with
  Status, Context, Decision, Consequences, Alternatives. To reverse a decision,
  add a new ADR and mark the old one `Superseded by NNNN`; never delete.
  Answering one of the spec's Appendix B open questions requires an ADR.
  Claim the number first: sessions run in parallel, so get it from
  `~/xo-lead/agents/adr-claim.sh SLUG WHO` (it records the claim under a
  lock and prints the next free number), then commit a stub
  `NNNN-title.md` (Status: Proposed) and its README line on main, then
  fill it in. `TestDecisionNumbersUnique`
  fails on a duplicate number.
- **Std change logs** (decision 0276): a user visible change to a std
  module (fix, port, behavior, security, feature, perf) adds an entry to
  `std/changes/MODULE.md` in the same commit, in 0276's format. Std
  follows Go's standard library (port Go's code and tests where they
  fit, with attribution; Xo's own API); Xo Lang Lib 1 owns std.
  Port Go's fast paths too, not only its general algorithm, and give
  every port a timing row against Go's package beside its corpus
  (owner, 2026-10-21: ports that matched Go's output ran 3 to 10x
  slower; gotchas/std-port-costly-go-idioms.md: Str/Bytes slicing
  copies, small Lists allocate, no package-level caches).
- **Gotchas**: when something breaks unexpectedly, behaves surprisingly, or
  costs more than one attempt to get right, add a gotcha with symptom, cause,
  and fix. Mark it `Observed` or `Anticipated`; flip anticipated entries to
  observed (or delete them if they turn out wrong) once hit.
- **Infrastructure**: when commands, layout, toolchain versions, dependencies,
  or CI change, update `knowledgebase/infrastructure/`. Put a
  `Last verified: YYYY-MM-DD` line on facts that can go stale.
- Every new file gets a one line entry in its folder's `README.md`.
- Use absolute dates (2026-10-07), not relative ones.
- Before finishing a task, check: did anything in this task change the spec,
  make a decision, reveal a gotcha, or change infrastructure? If yes, update
  the knowledgebase before reporting done.

## Conventions

- Write the language name as **Xo** in prose; lowercase `xo` only for the
  CLI, the `.xo` extension, `xo.mod`/`xo.lock`, and the `xo-lang` namespace.

- Prototype code: Go, standard library preferred, `gofmt`, table driven and
  golden file tests under `testdata/`.
- Tests (owner, 2026-10-09: run the minimum, fix forward):
  - each commit: `go test -short` over the repo (exclude `runs/`), about 10s;
  - before a merge or push: `-short` plus the full tests of only the
    packages the change touched (and `go vet` on them);
  - the full suite (`go test ./...` without `-short`) only before tagging a
    release, after a large merge (a new backend feature across packages),
    or from `bench/nightly.sh`; a failure it finds is fixed forward with a
    regression test, not by reverting.
  - merging an agent's branch (owner, 2026-10-11): if the merge is clean
    and main did not change the packages the agent tested since its run,
    `-short` plus `go vet` is enough; rerun full tests only for packages
    changed on both sides. Push first, then run any full suite in the
    background and fix forward. Start the next agent without waiting.
  - rebasing before a push (owner, 2026-10-16): do not rerun tests after
    a rebase that does not affect the work. Rerun only when the incoming
    commits change files the change touches or depends on (the same
    packages, or the self build's files when the change is in the fixed
    point set), or when resolving a conflict changed code; then rerun
    only those tests. A conflict only in docs (CHANGELOG, a README) needs
    no rerun.
  - agents run in parallel in separate worktrees (owner, 2026-10-11, on
    a larger plan); RAM is the limit, so every heavy test run (non
    -short tests of internal/selfhost, internal/gen, internal/llvm,
    internal/arm64, cmd/xo, self-hosted builds, benchmarks) goes through
    the lead's machine-wide lock script, one at a time, and each agent
    logs its steps to a file the lead monitors.
  - never wait idle (owner, 2026-10-19): while a gate, the push queue,
    a Docker run or a long build runs, an agent works on its next item
    in another worktree (or its measurements, docs, or the next step's
    design); after posting a status or report it continues straight to
    its next queued item instead of ending its turn. Only an empty queue
    stops it: then it asks the lead for the next item. The board's NEXT
    field always names one.
  - batched runs (owner, 2026-10-11, 2026-10-12): agents run no
    verification before reporting (no merge tier, no package suites),
    only `-short` and the targeted heavy runs their work depends on; the lead
    merges each report (`-short` and vet after each merge) and runs the
    merge tier and the full tier once on the combined main per batch, bisecting over the batch's
    merge commits when it fails, and the owning agent fixes forward.
  - heavy runs stay on the Mac (owner, 2026-10-20); it has 24 GB, so
    heavy.sh runs up to two heavy jobs, the second only when 60% of memory is free (HEAVY_MIN_FREE)
    (HEAVY_MIN_FREE). CI is used only with the owner's approval for that
    use: ask the lead, who asks the owner.
  - batch mode is standing (owner, 2026-10-20); per push only the bare
    minimum (owner, 2026-10-21): -short, the one or two tests that show
    the change works, the fixed point only when the change touches what
    compiler/xo itself builds from (compiler/, or std/native and llrt
    files it imports), and the 42-golden Docker amd64 sample only for an
    x86 code generation change. No package suites, no identity unless a
    push claims no output change, no extra benchmark sweeps unless the
    push claims a speedup. The lead runs the full tier every 8 hours
    (scripts/gate.sh short arm64 llvm cmd gen fp, LLVM at -O1 with Go
    parity, plus the Docker amd64 goldens), bisects a failure over that
    window's pushes, and the owner of the commit fixes forward.
  - per commit only the agent's gate (owner, 2026-10-21): pushq rebases
    and pushes with no -short (PUSHQ_SHORT=1 runs it) but runs the fast
    repo checks (a few seconds, no xo build: TestOutsideBytesValidated,
    TestStdIndexCurrent, TestDecisionNumbersUnique, TestSpecListsEveryCode,
    TestCodegenInputsClassified; owner, 2026-10-21), and no overlap
    gate (owner, 2026-10-21: the hourly run or the next agent's gate
    catches a bad combination); go test -short and the full fixed point
    run every 2 hours on main (~/xo-lead/agents/hourly.sh; owner,
    2026-10-22, was hourly), and the lead
    bisects a failure to the commit's owner, who fixes forward. An
    agent still runs any repo test its change obviously affects (e.g.
    TestCodegenInputsClassified for a new XO_* variable) in its gate.
  - a push that changes a default (an XO_* default, a flag default, a
    default-on pass or output) runs, in its gate, the full suites of the
    packages it touches (e.g. internal/arm64's cache tests, the dev
    goldens, cmd/xo), not only targeted tests; a change to a runtime
    limit or allocation behavior that xo itself runs gates on a self
    build with the newly built xo (scripts/bootstrap-xo.sh s2) (owner,
    2026-10-22: a defaults batch reached main with three failures, and a
    memory budget default stopped xo building itself).
  - every build, test, benchmark or repro an agent starts goes through
    heavy.sh (gate class for short ones), even a one-off (owner,
    2026-10-22: a direct xo build reached 37 GB and stalled the Mac);
    heavy.sh stops the job's process group past HEAVY_MAX_GB of memory
    (default 16, compressed pages counted) and logs why. macOS ignores
    `ulimit -v`.
  - gates take no slot (owner, 2026-10-21): run your gate as
    `HEAVY_CLASS=gate ~/xo-lead/agents/heavy.sh NAME CMD...`; it starts
    as soon as 55% of memory is free and fewer than 2 gates run (owner,
    2026-10-21; starts staggered by 20 s), with no queue. The two heavy
    slots are for hourly.sh, bulk runs and suites. A job the memory
    reaper stops is reported to the lead, who restarts it (owner,
    2026-10-21: no owner go needed any more).
  - code size per push (owner, 2026-10-21): a codegen change reports xo's
    native __text only; the LLVM column (split 12, no LTO) is measured
    in the lead's full tier for that window's codegen changes, since an
    LLVM build of xo takes about 25 minutes under load.
  - a failure found on main that is not yours (owner, 2026-10-21):
    message the lead first with the error and the good and bad
    commits; the lead assigns one bisect owner (two agents bisected one
    failure that day). An old xo (a bisect step, an identity check's
    base) runs with a private `XO_BACKEND_CACHE_DIR` deleted after, so
    its older prune never touches the shared cache (an old xo's prune
    evicted about 25 GB of it the same day).
  - short runs are gates (lead, 2026-10-21): a measurement or check
    expected to take under about 2 minutes runs as HEAVY_CLASS=gate, not
    bulk, so it never waits behind long bulk jobs (two 30 s measurements
    waited 45 minutes that day).
  - merges before testing (owner, 2026-10-21): heavy.sh serves pushq
    first, then gates, then HEAVY_CLASS=bulk runs (measurements,
    profiles, full suites, deferred checks; set it on every such run).
    When the lead declares merge-first (the file ~/xo-lead/merge-first
    exists: a long heavy queue), a gate is only -short's own tests, the
    one or two proving tests and the fixed point when required; every
    other check is appended with its exact command to
    ~/xo-lead/notes/deferred.md, and the lead runs one large test (the
    full tier plus the deferred list) when the queue drains, then
    removes the file. Agents keep working while they wait for a slot.
  - arm64 only (owner, 2026-10-14, replaces the platform runs at
    milestones): "testing on just the fastest machine and arch". Agents
    verify on the Mac (arm64); the full tier, when wanted, runs on one
    linux/arm64 EC2 machine (r8g, warm, seed prefix). Linux amd64 and
    Windows runs are best effort, not a gate: no milestone needs them,
    and a failure seen only there is recorded as a known issue, not
    chased by default. Exception (2026-10-20, two x86 miscompiles the
    arm64 gates could not see): a change to x86 code generation
    (compiler/arm64's x86*.xo and amd64 encoder, or anything only
    --backend=amd64 runs) also passes the per-push sample of the Docker
    amd64 goldens (scripts/docker-amd64-sample.txt, 42 of 201, about 15
    minutes; owner 2026-10-20) with every optional pass on, plus a run
    of the program it targets, before it pushes; the full 201 run in
    the lead's 8-hourly full tier. Every merge touching what the self build
    compiles or runs builds xo once, with the current xo (`go run
    ./scripts/xobuild`), and runs its proving tests with that xo: no
    fixed point per push (owner, 2026-10-21). The full self rebuild
    fixed point (scripts/bootstrap-xo.sh, s2 and s3 identical) runs
    every 2 hours on main (~/xo-lead/agents/hourly.sh, with go test -short; bisected to the
    owner of the guilty commit, who fixes forward) and in releases,
    seed-cut.sh and CI, so a miscompile cannot build the next released
    compiler. The paths: compiler/ (but not
    compiler/macho or compiler/wasm, which the bootstrap never uses,
    and not `_test.xo` files), std/native/, std/llrt/, and the xo build
    code (internal/xotool xotool.go, chain.go, seed.go, lock_*.go,
    scripts/xobuild, scripts/seed-chain.txt). Other merges (tests, docs,
    test-only tooling) need vet and -short only. A gate skips the full
    internal/llvm tests when every changed file is native backend only
    (compiler/arm64, compiler/macho, internal/arm64, docs, bench
    results; owner, 2026-10-18): scripts/gate.sh's llvm step does this,
    and otherwise runs only the LLVM goldens the change touches (owner,
    2026-10-22: the full package took 46 to 56 minutes per gate; the full
    set runs in the lead's full tier),
    XO_GATE_LLVM=1 forces it. internal/llvm's goldens build at -O1
    with no LTO except an -O2 sample (llvm_test.go o2Goldens; owner,
    2026-10-18); XO_TEST_LLVM_OPT=O2 builds every golden at -O2, for a
    change to what clang's optimizer sees (noalias, ranges, poison
    flags); a clang cache keyed on the IR (XO_CLANG_CACHE=off disables
    it) lets an unchanged IR skip clang; TestGoldenGoParity
    (XO_TEST_GO_PARITY=1, set by gate.sh for compiler/gen and std/rt)
    checks the Go backend against want.txt. The batch full tier runs
    the LLVM goldens at -O1 (with the -O2 sample) and
    XO_TEST_GO_PARITY=1 (owner, 2026-10-20: native is the default, so
    LLVM is the reference backend); only seed-final.sh (seeds and
    releases) sets XO_TEST_LLVM_OPT=O2.
    Long pass/fail runs go
    to on demand CI, not the Mac's single heavy slot.
  - CI (decision 0225, owner 2026-10-14): GitHub Actions on self-hosted
    linux/arm64 EC2 runners (infra/ci-runners), started only by hand,
    never on push: push the branch, then `gh workflow run ci.yml -f
    ref=BRANCH -f suite=auto` (short, the touched packages' full tests,
    the selfhost merge tier and cmd/xo, and the fixed point when the
    paths above changed, as parallel jobs in one run; tested merged with
    main), `gh run watch RUN_ID
    --exit-status --interval 10` (owner 2026-10-14: poll every 10
    seconds, never 30 or 60, so a fast failure reaches the session as
    soon as the owner sees it; run it in the background, or a Monitor
    loop that prints on a status change with `sleep 10` and stops on
    completed), `gh run view RUN_ID --log-failed`. Other suites:
    short, vet, merge (`-f packages=...`), fixed-point, full, windows
    (best effort, xo-windows runner), script (`-f packages="PATH
    ARG..."`: a .sh or .ps1 from the ref with XO_CLI, the checkout's
    and main's std, and an uploaded output directory), measure
    (`-f packages=<bench/perf dir>`: Linux instruction counts, main
    against the branch). This replaces the lead's EC2 queue. The Mac
    keeps Mac only checks and runs that write files into the worktree.
    Full reference: [ci.md](knowledgebase/infrastructure/ci.md).
  - internal/selfhost has tiers (`XO_SELFHOST_TIER`): `short` (-short),
    `merge` (the default, about 4 min on the Mac), `full` (every backend,
    58 min cold on r8g.8xlarge, `-timeout 120m`, on demand: `gh workflow
    run ci.yml -f suite=full -f runner=xo-arm64-big`,
    knowledgebase/infrastructure/ci.md); `XO_SELFHOST_JOBS` sizes its
    worker pool (default min(GOMAXPROCS, 4)); full runs before tagging and
    for a change under compiler/ or the compiler's SSA or native backends
    that the merge tier does not exercise.
- The Go compiler is gone (decision 0215, 2026-10-13; the last commit
  with it is tagged `go-compiler-final`, 87873aa5): `xo` is xo1, built
  from compiler/xo, and the Xo implementation (compiler/, std/) plus the
  spec are the source of truth. Every compiler change (language,
  backends, targets, driver) lands in compiler/. What stays in Go: std/rt
  (the Go backend's runtime), internal/gobind and cmd/xo-gobind (`use
  go`), internal/gotool, the test infrastructure (internal/xotool,
  progen, testtmp, mod/modtest), and test only packages (cmd/xo,
  internal/llvm, gen, arm64, selfhost, repotest) that drive the xo
  binary.
- The xo the Go tests and scripts run (internal/xotool): `$XO_CLI`, else
  the checkout's `bin/xo`, which `go run ./scripts/xobuild` (and any Go
  test that needs it) rebuilds when compiler/ or a std module it uses
  changed: several minutes and several GB, once per source key, shared
  by worktrees in `<user cache>/xo/checkout/<key>`. An earlier xo builds
  it (`XO_SEED`, the earlier bin/xo, the newest cached build of an
  ancestor commit, `xo` on
  PATH, else the xo1 of the go-compiler-final tag, built once with Go,
  through the hops of scripts/seed-chain.txt, decision 0216).
  Set `XO_CLI` to a built xo to skip rebuilds while iterating.
- Seeds (decision 0216, owner 2026-10-13): release N builds with release
  N-1. Prebuilt seeds (a tarball of bin/xo plus its std, since xo finds
  std at bin/../std, per goos-goarch, with a .sha256) live in the PRIVATE
  S3 bucket named by `$XO_SEED_BUCKET`, else the first line of
  `<user config dir>/xo/seed-bucket` (us-east-1, public access blocked,
  versioned; the repository names no bucket or account, decision 0249:
  the values are in the untracked CLAUDE.local.md, and xobuild with
  neither, and no XO_SEED_URL, fetches no seed): `seeds/<version>/<goos>-<goarch>.tar.gz`,
  `seeds/<version>/manifest.txt`, `seeds/latest.txt`. Seeds are not
  public releases. This Mac's AWS user and EC2 instances (instance
  profile `xo-ci-seed-read`, read only on that bucket) download them
  with the aws CLI; xobuild fetches the newest seed before falling back
  to the go-compiler-final root and its chain. When a change lands that
  older seeds cannot build (new runtime intrinsics the compiler's own
  code uses, gotchas/seed-older-than-intrinsics.md), the minor version
  (`xo version`) is bumped and a new seed cut for every platform instead
  of adding a chain hop: `scripts/seed-cut.sh VERSION NOTES WORKDIR`
  then `scripts/seed-upload.sh WORKDIR` on the development Mac
  (toolchain.md "Cutting a seed"). Any session may run them, but only
  after the owner approves that version (owner, 2026-10-14); IAM changes
  stay with the lead, agents never make them. If a seed cannot be fetched, stop and report the
  error (owner, 2026-10-13): never fall back to rebuilding from
  go-compiler-final through the chain (25 to 37 minutes); that path is
  opt in, for the replay from source and a brand new platform. Unpack a
  seed into its own directory (its top directory is xo/, the EC2
  checkout is ~/xo).
- The TestSelfhost* dump replays (lexer, parser, format, checker, SSA,
  arm64, LLVM, gen, defhash, mutations; recordings in
  internal/selfhost/testdata/gogoldens) are skipped by default
  (decision 0217, owner 2026-10-13: no more 25 to 40 minute
  re-recordings); `XO_SELFHOST_DUMPS=all` runs them for a deliberate
  audit. The compiler is verified by behavior: program output goldens
  on every backend, the fixed point, error check and codegen pattern
  tests, progen across backends, and xo unit tests. Never re-record
  dumps to get a change merged.
- Diagnostics are JSON with stable `XOnnnn` codes (spec section 10). Never
  change the meaning of an existing code; add a new one.
- Generated Go must carry `//line` directives back to `.xo` sources.
