# What mature languages have that Xo does not (yet)

Status: a list for the owner to revisit (owner, 2026-10-27: "Document
them, I'll bring them up again later"). Not decided or scheduled; a
larger standard library is tracked separately and left out here.

Written after 0.33.0. Checked against the spec and the knowledgebase
first: Xo already has versioned module requires with capability limits
(spec core, `require ... uses ...`), an LSP, a debugger, a profiler, a
race detector, fuzzing, a formatter, vet, `xo explain`, signed release
archives and byte-identical (reproducible) builds. Items marked
*unverified* have a mention somewhere in the repo that was not checked
in detail; check before starting one.

## Highest value

1. **A stability promise.** A published compatibility policy, as Go's 1
   compatibility guarantee or Rust's editions: what may change between
   versions, how deprecations work (0.33.0's uuid `u.0` deprecation then
   removal is the first instance), and how long a deprecated form keeps
   working. Xo has `edition preview` but no written policy. The owner
   decides when 1.0 is; this is the step before it.
2. **A package registry and module discovery.** Xo modules can be
   required by version, but there is no index to find them, and no module
   proxy or checksum database (Go's proxy.golang.org and sum.golang.org)
   that makes dependencies tamper-evident and available when the origin
   is gone. *Verified 2026-10-29:* the client side exists in Xo (`XO_PROXY`
   and `xo mod proxy serve`, decision 0126; the checksum log client,
   `XO_SUMDB`, decision 0124, in compiler/cli/modules); no public proxy or
   log runs, and there is no index.
3. **A security process.** A vulnerability database, a `xo vuln` checker
   for a module's dependencies (as `govulncheck`), and a published
   security policy (how to report, response times).
4. **Documentation for newcomers.** A guided tour or book, and browsable
   std API documentation (as pkg.go.dev). The spec and `xo doc` are
   reference material, not a way in.
   *Verified 2026-10-29:* the website has a 12-page tour whose examples
   are checked and run in the playground (content/docs/tour, last edited
   2026-10-17); browsable std API docs do not exist (`xo doc` writes the
   Markdown they could be made from).

## Important, smaller

5. **Editor support.** A published VS Code extension and a tree-sitter
   grammar, so Xo highlights in editors and on GitHub. *Verified
   2026-10-29:* editors/vscode is a complete extension (TextMate grammar,
   an `xo lsp` client; decision 0113) that is not published; no tree-sitter
   grammar exists.
6. **More platforms.** FreeBSD and Linux riscv64 are the common asks
   after the five current targets; 32-bit and mobile matter less for a
   backend language.
7. **Database drivers beyond Postgres:** SQLite and MySQL.
8. **Observability:** OpenTelemetry-style tracing and metrics in std.
   *Verified 2026-10-29:* std/metrics
   (Prometheus exposition) and std/trace (request ids, a span ring) exist
   (decision 0325, spec 11.6); OpenTelemetry export does not.
9. **Calling Xo from other languages:** building a C-compatible shared
   library, as Go's `-buildmode=c-shared`. *Verified 2026-10-29:* none for
   users; only the runtime's own C-name exports (decisions 0207, 0234).

## Process

10. **A public process:** a proposal process for language changes (the
    ADRs are most of it, without a public front door), a release cadence,
    and possibly a long-term-support line.

## Lead's recommendation (2026-10-27)

The stability policy (1), the registry with a checksum database (2) and
the tour or book (4) decide whether people can adopt Xo, more than
further speedups. The *unverified* items were checked on 2026-10-29 (the
notes above): a stability policy has spec 13 and decisions 0089 and 0155
behind it, and a 12-page tour runs in the playground on the website.

## Related lessons from the 0.33.0 testers

Both testers' new bugs were in `use go` workflows (install, a fresh
clone, `mod tidy`, the newest Go), not the compiler. Tests of the
workflows a new user hits first (release smoke with `use go` and a
fresh-clone tidy, a newest-Go check) are milestone 0.34.0 work.
