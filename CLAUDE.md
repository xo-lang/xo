# Xo

Xo is a small, statically typed language for backends and web services,
designed for AI agents to write, check, cross compile, and debug. The P1
prototype is a compiler written in Go that emits Go source.

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
- Diagnostics are JSON with stable `XOnnnn` codes (spec section 10). Never
  change the meaning of an existing code; add a new one.
- Generated Go must carry `//line` directives back to `.xo` sources.
