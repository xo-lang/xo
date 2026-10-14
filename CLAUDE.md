# Xo

Xo is a small, statically typed language for backends and web services,
designed for AI agents to write, check, cross compile, and debug. The
compiler and the `xo` toolchain are written in Xo (selfhost/); the Go
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

- `spec/core.md`: the language specification. Source of truth for syntax and
  semantics.
- `spec/std.md`: prelude methods and standard library surface.
- `knowledgebase/infrastructure/`: toolchain, repo layout, build and test
  commands, CI, namespaces (GitHub org, Homebrew, etc.).
- `knowledgebase/decisions/`: design decisions as numbered ADRs
  (`NNNN-short-title.md`).
- `knowledgebase/gotchas/`: traps and surprises, with how to avoid them.
- `bench/`: benchmark tasks and results that decide the `Proposed` decisions.
  `bench/paper/` holds solutions written blind from the spec; do not edit them.

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
  Claim the number first: sessions run in parallel, so before starting
  work in a branch, commit a stub `NNNN-title.md` (Status: Proposed) and
  its README line on main, then fill it in. `TestDecisionNumbersUnique`
  fails on a duplicate number.
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
  - agents run in parallel in separate worktrees (owner, 2026-10-11, on
    a larger plan); RAM is the limit, so every heavy test run (non
    -short tests of internal/selfhost, internal/gen, internal/llvm,
    internal/arm64, cmd/xo, self-hosted builds, benchmarks) goes through
    the lead's machine-wide lock script, one at a time, and each agent
    logs its steps to a file the lead monitors.
  - batched runs (owner, 2026-10-11, 2026-10-12): agents run no
    verification before reporting (no merge tier, no package suites),
    only `-short` and the targeted heavy runs their work depends on; the lead
    merges each report (`-short` and vet after each merge) and runs the
    merge tier and the full tier once on the combined main per batch, bisecting over the batch's
    merge commits when it fails, and the owning agent fixes forward.
  - platform runs at milestones (owner, 2026-10-13): agents verify on
    the Mac only (targeted heavy runs, `-short`); Linux and Windows runs
    happen at milestones (after a stage of runtime or compiler work),
    run by the lead on EC2, and failures are bisected over the
    milestone's merges (one targeted test per probe). Code that cannot
    run on the Mac (Linux-only, Windows-only paths) gets one batched
    platform run per agent before it reports. Every merge touching the
    compiler or the runtime also passes the self rebuild fixed point
    (scripts/bootstrap-xo.sh, s2 and s3 identical) on the Mac, so a
    miscompile cannot build the next compiler.
  - internal/selfhost has tiers (`XO_SELFHOST_TIER`): `short` (-short),
    `merge` (the default, about 4 min on the Mac), `full` (every backend,
    58 min cold on r8g.8xlarge through scripts/remote-full.sh, `-timeout
    120m`); `XO_SELFHOST_JOBS` sizes its worker pool (default
    min(GOMAXPROCS, 4)); full runs nightly, before tagging, and
    for a change under selfhost/ or the compiler's SSA or native backends
    that the merge tier does not exercise.
- The Go compiler is gone (decision 0215, 2026-10-13; the last commit
  with it is tagged `go-compiler-final`, 87873aa5): `xo` is xo1, built
  from selfhost/xo, and the Xo implementation (selfhost/, std/) plus the
  spec are the source of truth. Every compiler change (language,
  backends, targets, driver) lands in selfhost/. What stays in Go: std/rt
  (the Go backend's runtime), internal/gobind and cmd/xo-gobind (`use
  go`), internal/gotool, the test infrastructure (internal/xotool,
  progen, testtmp, mod/modtest), and test only packages (cmd/xo,
  internal/llvm, gen, arm64, selfhost, repotest) that drive the xo
  binary.
- The xo the Go tests and scripts run (internal/xotool): `$XO_CLI`, else
  the checkout's `bin/xo`, which `go run ./scripts/xobuild` (and any Go
  test that needs it) rebuilds when selfhost/ or a std module it uses
  changed: several minutes and several GB, once per source key, shared
  by worktrees in `<user cache>/xo/checkout/<key>`. An earlier xo builds
  it (`XO_SEED`, the earlier bin/xo, the newest cached build, `xo` on
  PATH, else the xo1 of the go-compiler-final tag, built once with Go,
  through the hops of scripts/seed-chain.txt, decision 0216).
  Set `XO_CLI` to a built xo to skip rebuilds while iterating.
- The TestSelfhost* tests compare each self hosted component with a
  recording (internal/selfhost/testdata/gogoldens, the Go
  implementation's output until decision 0215). A change that alters a
  component's output on purpose, or a new or changed input with no
  recording, needs `scripts/selfhost-record-goldens.sh` (record: units
  with no recording of their input; `XO_SELFHOST_GOLDENS=rerecord` for
  every unit a run computes) and a look at the recording's diff in the
  same change.
- Diagnostics are JSON with stable `XOnnnn` codes (spec section 10). Never
  change the meaning of an existing code; add a new one.
- Generated Go must carry `//line` directives back to `.xo` sources.
