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
   is gone. *Unverified:* a few documents mention a proxy.
3. **A security process.** A vulnerability database, a `xo vuln` checker
   for a module's dependencies (as `govulncheck`), and a published
   security policy (how to report, response times).
4. **Documentation for newcomers.** A guided tour or book, and browsable
   std API documentation (as pkg.go.dev). The spec and `xo doc` are
   reference material, not a way in.

## Important, smaller

5. **Editor support.** A published VS Code extension and a tree-sitter
   grammar, so Xo highlights in editors and on GitHub. *Unverified:* some
   mentions exist.
6. **More platforms.** FreeBSD and Linux riscv64 are the common asks
   after the five current targets; 32-bit and mobile matter less for a
   backend language.
7. **Database drivers beyond Postgres:** SQLite and MySQL.
8. **Observability:** OpenTelemetry-style tracing and metrics in std.
   *Unverified:* some metrics mentions exist.
9. **Calling Xo from other languages:** building a C-compatible shared
   library, as Go's `-buildmode=c-shared`. *Unverified:* one mention.

## Process

10. **A public process:** a proposal process for language changes (the
    ADRs are most of it, without a public front door), a release cadence,
    and possibly a long-term-support line.

## Lead's recommendation (2026-10-27)

The stability policy (1), the registry with a checksum database (2) and
the tour or book (4) decide whether people can adopt Xo, more than
further speedups. A milestone proposal would verify the *unverified*
items first.

## Related lessons from the 0.33.0 testers

Both testers' new bugs were in `use go` workflows (install, a fresh
clone, `mod tidy`, the newest Go), not the compiler. Tests of the
workflows a new user hits first (release smoke with `use go` and a
fresh-clone tidy, a newest-Go check) are milestone 0.34.0 work.
