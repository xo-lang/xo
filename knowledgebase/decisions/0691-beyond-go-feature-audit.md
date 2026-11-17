# 0691: a std module no Go package maps to reaches features Proven through a committed feature audit

Status: Accepted (owner, 2026-11-16, option 2 of task #188; written by Xo Lang 2; amends 0461 and 0655)

## Context

Decision 0461 gives each std module a features mark, and Proven reads "near full parity, or beyond Go".
Decision 0655 made Proven need every coverage row naming the module done and no open stdaudit gap in
the Go packages of those rows. A module that no row of `knowledgebase/infrastructure/std-coverage.md`
names has nothing to compare: scripts/stdcov's `feat` returned Solid for it ("no Go package maps to
it", or "beyond Go: ..." for the modules in its `beyondGo` map) and nothing could move it higher. On
2026-11-16 that module is std/ui (decisions 0659 to 0689), which has no Go counterpart at all. The
modules of the `beyondGo` map (sql, websocket, toml, uuid, rate, backoff, metrics, trace, dns, udp) all
have coverage rows by now and are scored through those rows and their 0655 audits, so the map only
names them.

The owner chose (2026-11-16, through the lead) to give such a module a Proven path of its own: a
committed list of its features, each with a test.

## Decision

1. **The feature audit.** A std module that no coverage row names may commit
   `std/evidence/MODULE/features.md`, beside its other Proven evidence (decision 0616). Free text and
   headings are allowed; scripts/stdcov reads only the item lines:

       - [x] FEATURE | DECISION | TEST
       - [ ] FEATURE | DECISION | what is open

   FEATURE is a short name; DECISION names the ADR (or spec section) that defines it; TEST is a repo
   path that exists (a golden directory, a test file), optionally followed by `:Name` for the test in
   it, in backquotes or not. The list is every feature the module's own ADRs and its spec/std.md section
   promise, not only the finished ones: a promised feature that is not built is a `- [ ]` item.
2. **Scoring.** For a module with no coverage row, stdcov's `feat` reads the file. Proven: at least one
   item, no `- [ ]` item, and every `- [x]` item names a TEST path that exists. Otherwise Solid, and the
   note counts the tested features and names the open ones and the done ones without a test (the first
   four, then a count), linking the file. With no file the mark stays Solid and says "no feature audit
   yet (decision 0691)". TestFeatAudit in scripts/stdcov holds it.
3. **Modules with coverage rows keep decision 0655.** A feature audit does not replace a module's Go
   rows or its stdaudit gaps; a module that has rows is scored by them alone, so the `beyondGo` modules
   need no feature audit.
4. **Keeping it current.** A change that adds, removes, or finishes a feature of such a module updates
   its features.md in the same commit, as 0655 item 2 does for map.txt. The first audit is std/ui's
   (task #186, xo-4c).

## Consequences

- std/ui can reach features Proven, and its open items become a work queue in one file.
- The check that a test exists is by path only: stdcov does not run the test. The module's own test
  run, the goldens, and the evidence of decision 0616 keep it passing.
- A list that leaves a promised feature out would read Proven; the 0616 review (and the reviewer of the
  commit that adds the file) checks the list against the module's ADRs.

## Alternatives

- Keep the Solid cap for every module without a Go counterpart (option 1): std/ui could never be
  Proven, though nothing in Go is there to compare it with.
- A row for each such module in std-coverage.md with a hand written Missing list: the same list without
  a test per feature, and the hand written Status is what 0655 moved away from.
- Map the module to a third-party Go package (a web framework) and audit its symbols: no package has
  std/ui's design, so the gaps would be noise.
