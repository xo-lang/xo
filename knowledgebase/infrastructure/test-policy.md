# Test and push policy: the full record

Moved here from CLAUDE.md on 2026-11-10 (owner: agents now start fresh
after every task, so CLAUDE.md keeps only the current rules and points
here). This is the dated record of how the test and push policy grew,
kept verbatim; where it disagrees with CLAUDE.md, CLAUDE.md is current.
Commands and limits: agent-tools.md "Testing" and "Coordinating",
offload.md. Last verified: 2026-11-10.

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
  - heavy runs stay on the Mac (owner, 2026-10-20); since 2026-11-09 it is a
    Mac Studio (30 cores, 96 GB; owner: "raise the limits"), so heavy.sh runs up
    to four heavy jobs (HEAVY_SLOTS), each stopped past 32 GB (HEAVY_MAX_GB),
    the second to fourth only when 40% of memory is free (HEAVY_MIN_FREE)
    (HEAVY_MIN_FREE). CI is used only with the owner's approval for that
    use: ask the lead, who asks the owner.
  - batch mode is standing (owner, 2026-10-20); per push only the bare
    minimum (owner, 2026-10-21): -short, the one or two tests that show
    the change works, the fixed point only when the change touches what
    compiler/xo itself builds from (compiler/, or std/native and llrt
    files it imports), and the amd64 sample on the linux/amd64 offload
    host only for an x86 code generation change or a change to the C
    compiler or linker flags xo passes (owner, 2026-10-31: a clang only
    flag broke every gcc build on Linux, b87f61855; the amd64 host's cc is
    gcc). A push changing what compiler/xo builds from (compiler/ but
    compiler/macho, compiler/wasm and `_test.xo`; std/native; std/llrt)
    also builds compiler/xo once, cold, with the new xo under
    XO_MAX_MEM=8G with --phase-times (owner, 2026-11-06: an inliner
    runaway past 22 GB reached main); scripts/gate.sh runs it and pushq
    requires "selfbuild" in PUSHQ_RAN. No package suites, no identity unless a
    push claims no output change, no extra benchmark sweeps unless the
    push claims a speedup. The lead runs the full tier every 8 hours
    (scripts/gate.sh short arm64 llvm cmd gen fp, LLVM at -O1 with Go
    parity, plus every amd64 golden on the offload host), bisects a failure over that
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
    as soon as 35% of memory is free, fewer than 5 gates run and the load
    is under 48 (owner, 2026-11-09; was 2 gates and load 16 on the 24 GB
    Mac, then 28, which queued gates at the Studio's normal load; starts staggered by 20 s), with no queue. The heavy
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
    A bulk run on the Mac also sets HEAVY_DARWIN=1, since only runs that
    need darwin stay on the Mac; heavy.sh refuses other bulk runs (exit
    64) and they go to an offload host (owner, 2026-10-30).
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
    --backend=amd64 runs) also passes the per-push amd64 sample
    (scripts/docker-amd64-sample.txt) with every optional pass on, plus a
    run of the program it targets, before it pushes; the full set runs in
    the lead's 8-hourly full tier. The sample runs natively on the
    owner's linux/amd64 offload host (owner, 2026-10-29, replacing the
    Docker run on the Mac: 33 s against about 15 minutes): commit, then
    `~/xo-lead/agents/amd64-run.sh REV NAME sample` (no heavy.sh slot;
    the host runs one job at a time; infrastructure/offload.md). Every merge touching what the self build
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
    to the offload hosts, not the Mac's single heavy slot.
  - CI retired (decision 0423, owner 2026-10-29): the EC2 runners and
    the GitHub runners are deleted, so `gh workflow run ci.yml` has no
    runner. Heavy runs go to the owner's offload hosts
    (knowledgebase/infrastructure/offload.md): linux/arm64 through
    `~/xo-lead/agents/offload.sh`, linux/amd64 through
    `~/xo-lead/agents/amd64-run.sh`, Windows on the amd64 host (to be
    scripted). They exist to take load off the Mac (owner, 2026-10-29):
    any test, build or measurement may run there without asking, one
    job at a time per host (a lock queues the rest). The old CI, for
    history:
  - CI (decision 0225, owner 2026-10-14, superseded): GitHub Actions on self-hosted
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
    58 min cold on r8g.8xlarge, `-timeout 120m`, on demand on an
    offload host through the lead); `XO_SELFHOST_JOBS` sizes its
    worker pool (default min(GOMAXPROCS, 4)); full runs before tagging and
    for a change under compiler/ or the compiler's SSA or native backends
    that the merge tier does not exercise.
