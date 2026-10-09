# 0095: Import direction rules (architecture tests)

Status: Proposed (2026-10-09)

## Context

Benchmark task 08 (notes service) asks for packages `domain`, `store`,
`api`, and `server` with dependency rules, and says "a test must fail if
`domain` imports `store` or `api`, or `store` imports `api`". Xo has no way
to state or check that. In the Codex r2 rerun
(`bench/results/codex-xo-r2-2026-10-09.md`) the first run of task 08 passed
`xo check` but its architecture test faulted: it read an invented
`XO_SOURCE_ROOT` variable, because a test's `os.fs` is a `MemFs` that
cannot read the checkout, and then wrote the "sources" into the fake file
system itself, so the test could not have caught anything. The second run
replaced it with a test that imports all four packages and relies on
cycle detection: a forbidden import creates a cycle only when the reverse
edge already exists (true for this layout today, false for a rule such as
"`domain` imports nothing from the project" once an unrelated package is
added). In Go the usual
answer is a test that runs `go list -deps` or parses imports with
`go/parser`; Xo tests cannot run commands or read real files on purpose
(core 9, std.md 8), so that habit has no Xo form.

What the toolchain already knows: the checker loads the whole import
graph of every module it checks (core 4.1: cycles are an error; 4.2:
`XO1306` rejects an import of a module that is not required), so a rule
over that graph costs nothing extra to evaluate at `xo check` time.

## Options

1. **std/testing API over the import graph.** `testing.imports(pkg: Str)
   -> List[Str]` (direct imports) and `testing.deps(pkg)` (transitive),
   filled in by `xo test` at build time from the checker's graph, so a
   test is ordinary Xo:
   `expect(testing.deps("example.com/notes/domain").any(fn(p) { p.starts_with("example.com/notes/") })) == false`.
   + Rules are code, any shape (allow lists, counts, regexes).
   + Fits the task text literally ("a test must fail").
   - Only checked when tests run; the error names a test, not the import
     line, and carries no fix.
   - The graph is a compile time constant baked into a test binary; a
     package not reached by the test's own module would need special
     loading.
   - Every project re-implements the same three line rule.
2. **`xo query imports <pkg>` (and `deps`).** JSON of the direct or
   transitive imports with the `use` line of each, next to the existing
   `xo query api`, `callers`, `effects`.
   + Cheap (one more query kind over data `xo serve` already has); useful
     to agents exploring a codebase and to CI scripts.
   - Not a test: Xo tests cannot run commands, so on its own it does not
     answer task 08. It moves the rule into shell scripts outside Xo.
3. **Declarative `deny` rules in xo.mod, checked by `xo check`.**
   ```
   deny example.com/notes/domain -> example.com/notes/store, example.com/notes/api
   deny example.com/notes/store -> example.com/notes/api
   ```
   Paths match a package and, with a trailing `/...`, everything under
   it (`deny example.com/notes/domain -> example.com/notes/...`). A `use`
   that a rule forbids, directly, is a new error `XO1312` at the `use`
   line, naming the rule and its xo.mod line; transitive edges are not
   checked (each direct edge is, so a chain is caught at its first
   forbidden step). Rules apply to the main module's own packages;
   dependencies' rules are checked when they are built as main modules.
   + Fails at check time, on every `xo check`, `xo serve`, and editor
     save, at the offending line: the earliest feedback, which is what
     the agent loop runs first.
   + One place, readable by agents and humans; no test code to get
     wrong; works for packages no test imports.
   - Grows the xo.mod grammar (one line kind) and the code space (one
     code). A rule is not a test, so a task that literally says "a test
     must fail" is met by `xo test` running the check first (it does:
     `xo test` checks before it builds), not by a test block.
   - Only deny rules; allow lists ("`domain` imports only std") need the
     `/...` form on the project root, which expresses them for the
     common case.
4. **Status quo: import cycles as the check.** Works only when every
   forbidden edge would close a cycle; silent otherwise. Rejected as the
   answer, but it is what agents will write until one of the above
   exists.

## Recommendation

Option 3, with option 2 as a small companion: `deny` lines in xo.mod
checked by the checker (`XO1312`, no machine fix: removing a `use` is a
design change, not a rewrite), and `xo query imports` so an agent can see
the graph and the rule that blocks an edge. Option 1 is deferred: once
rules are declarative, a test API adds a second place for the same fact;
revisit if a real project needs rules that `deny` cannot express. The
spec text would go in core 4.2 (xo.mod lines), 10.3 (query kinds), and
10.6 (code), with a golden under `internal/check/testdata` for a direct
forbidden import, a `/...` pattern, an unused but present `use`, and a
rule naming a package that does not exist (`XO1301`). Task 08's prompt
would then point at `deny` the way it points at other spec sections.

## Consequences (if accepted)

- Task 08's rule becomes two xo.mod lines; its first runs stop depending
  on test tricks, which removes the last r2 first run failure that had
  no diagnostic.
- xo.mod gains a line kind that `xo mod tidy` must keep and `xo fmt`
  (if it ever formats xo.mod) must order stably.
- A forbidden `use` that is also unused reports `XO1312`, not just the
  unused import warning, so the rule cannot be hidden by deleting the
  last call.

## Alternatives

Listed as options 1, 2, and 4 above. Also considered: rules in a separate
file (`xo.rules`), rejected because xo.mod already holds the module's
build facts and agents read it first; and per package `// deny`
comments in source, rejected because a rule about a package should not
live inside the package it constrains, where the change that breaks it
would also delete it.
