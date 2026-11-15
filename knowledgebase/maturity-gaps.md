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
   working. *Written 2026-11-08:* decision 0567 states what 0.x promises
   today and what 1.0 will promise (spec 13), with Road to 1.0 entry
   criteria for the owner to edit; it is frozen when the owner declares
   1.0, and the website page and post are with the owner for review.
2. **A package registry and module discovery.** Xo modules can be
   required by version, but there is no index to find them, and no module
   proxy or checksum database (Go's proxy.golang.org and sum.golang.org)
   that makes dependencies tamper-evident and available when the origin
   is gone. *Verified 2026-10-29:* the client side exists in Xo (`XO_PROXY`
   and `xo mod proxy serve`, decision 0126; the checksum log client,
   `XO_SUMDB`, decision 0124, in compiler/cli/modules); no public proxy or
   log runs, and there is no index. *Updated 2026-11-08:* the log server
   is Xo again (`xo mod sumdb keygen` and `xo mod sumdb serve`, decision
   0124's addendum), and cmd/xo TestChecksumLog runs the proxy and the
   log end to end on loopback (fetch, verify, lock; XO1313 and XO1304 on
   a tampered tree; XO_NOSUMDB and XO_NOPROXY), so what a public service
   would run is built and tested; hosting it is the owner's decision (decision 0568, Proposed: R2 and Cloudflare's CDN, the owner's OCI host).
   *Last verified 2026-11-15:* decision 0568 is Accepted (owner,
   2026-11-08) and the services run: proxy.xolang.org serves module
   versions (github.com/xo-lang/aws lists v0.1.0 to v0.2.1),
   sum.xolang.org/latest answers, and index.xolang.org/index lists
   modules; pkg.xolang.org (item 4) is the discovery page.
3. **A security process.** A vulnerability database, a `xo vuln` checker
   for a module's dependencies (as `govulncheck`), and a published
   security policy (how to report, response times). *Last verified
   2026-11-15:* SECURITY.md (how to report) is in the repository since
   2026-10-17, and each CHANGELOG security line names the affected
   versions; there is no vulnerability database and no `xo vuln`.
4. **Documentation for newcomers.** A guided tour or book, and browsable
   std API documentation (as pkg.go.dev). The spec and `xo doc` are
   reference material, not a way in.
   *Verified 2026-10-29:* the website has a 12-page tour whose examples
   are checked and run in the playground (content/docs/tour, last edited
   2026-10-17); browsable std API docs do not exist (`xo doc` writes the
   Markdown they could be made from). *Last verified 2026-11-15:*
   browsable docs exist: pkg.xolang.org serves module and std API docs
   (as pkg.go.dev), and the website has a std reference at
   xolang.dev/docs/stdlib. pkg.xolang.org's std docs are at v0.46.0,
   behind the 0.51.1 release, so they need a refresh step per release.

## Important, smaller

5. **Editor support.** A published VS Code extension and a tree-sitter
   grammar, so Xo highlights in editors and on GitHub. *Verified
   2026-10-29:* editors/vscode is a complete extension (TextMate grammar,
   an `xo lsp` client; decision 0113) that is not published; no tree-sitter
   grammar exists. *2026-11-05:* the extension moved to its own repository,
   xo-lang/vscode, and debugs through `xo debug --dap` (decision 0516).
   *Verified 2026-11-08:* published on 2026-11-05 as "Xo Language"
   (`xo-lang.xo`, version 0.1.1) on the VS Code Marketplace (verified
   publisher) and on Open VSX; the repository is public. *2026-11-08:*
   a tree-sitter grammar, `tree-sitter-xo` (infrastructure/
   tree-sitter-xo.md), is public at https://github.com/xo-lang/tree-sitter-xo;
   the npm package, the crate and the editor-list submissions await the
   owner's approval. *Last verified 2026-11-15:* Open VSX has
   `xo-lang.xo` 0.1.3 (2026-11-11); npm has no `tree-sitter-xo` package.
6. **More platforms.** FreeBSD and Linux riscv64 are the common asks
   after the five current targets; 32-bit and mobile matter less for a
   backend language. *Last verified 2026-11-15:* both are in: FreeBSD
   (decision 0573, Accepted; amd64 and arm64) and linux/riscv64 through
   LLVM (decision 0571, Accepted), in the 0.51.1 CHANGELOG.
7. **Database drivers beyond Postgres:** SQLite and MySQL. *Last
   verified 2026-11-15:* std/sqlite exists (decision 0534, Accepted;
   in 0.51.1, FreeBSD too); there is no MySQL driver.
8. **Observability:** OpenTelemetry-style tracing and metrics in std.
   *Verified 2026-10-29:* std/metrics
   (Prometheus exposition) and std/trace (request ids, a span ring) exist
   (decision 0325, spec 11.6); OpenTelemetry export does not. *Last
   verified 2026-11-15:* std/otlp exports OpenTelemetry traces and
   metrics (decision 0539, Accepted and implemented; in 0.51.1).
9. **Calling Xo from other languages:** building a C-compatible shared
   library, as Go's `-buildmode=c-shared`. *Verified 2026-10-29:* none for
   users; only the runtime's own C-name exports (decisions 0207, 0234).
   *Last verified 2026-11-15:* `xo build --lib=shared` builds C shared
   libraries, Windows DLLs with their import library too (decision
   0574, Accepted; in the 0.51.1 CHANGELOG).

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
