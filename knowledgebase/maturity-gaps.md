# What mature languages have that Xo does not (yet)

Status: a list for the owner to revisit (owner, 2026-10-27: "Document
them, I'll bring them up again later"; restructured 2026-11-15 at the
owner's request into Remaining and Done, each item with the check that
proves it). A larger standard library is tracked separately
(knowledgebase/infrastructure/std-coverage.md) and left out here.

## Keeping it current

Every item names its evidence as `Probe:` lines that
`scripts/lead/maturity-probes.sh` runs against what is live (the
registries, the services, the release archives, the published site) and
the checkout's HEAD. A Remaining item's probes pass while the gap is
there, so a DRIFT on one means the gap may be closed; a Done item's
DRIFT means it broke or moved. Run it at each release (release
CHECKLIST) and before editing this file:

    XO_CLI=bin/xo scripts/lead/maturity-probes.sh

Check what is live, never only code or older notes. Each item carries
the date its probes last passed.

## Remaining

1. **A vulnerability database and `xo vuln`** (as vuln.go.dev and
   govulncheck): an OSV feed for std and the xo-lang modules, and a
   checker of a module's xo.lock and toolchain against it. Owner
   priority 2026-11-15 (after 0.52): ADR 0647 (format, hosting on
   vuln.xolang.org, the CLI and its codes XO1321 to XO1323). `xo vuln`
   stage A is on main (2026-11-15), not in a release yet; the reports
   repository xo-lang/vulndb and vuln.xolang.org wait on the owner. Until
   then each CHANGELOG security line names the affected versions. The
   probes check the released xo and the live host. Last verified 2026-11-15.
   Probe: `help unknown[[:space:]]command vuln`
   Probe: `!http 200 https://vuln.xolang.org/`
2. **A security page on xolang.dev.** SECURITY.md (how to report) is in
   the repository since 2026-10-17; the site has no page with it,
   response times, supported versions and past advisories. Owner
   priority with item 1. Last verified 2026-11-15.
   Probe: `http 200 https://raw.githubusercontent.com/xo-lang/xo/main/SECURITY.md`
   Probe: `http 404 https://xolang.dev/security/`
3. **A MySQL driver.** Postgres and SQLite are in std; MySQL is not.
   Last verified 2026-11-15.
   Probe: `!file std/mysql.xo`
   Probe: `http 404 https://pkg.xolang.org/std/mysql/`
4. **The windows/arm64 release archive** (decision 0569 stage 6). The
   native backend builds windows/arm64 by default since 2026-11-09
   (`--targets=all` is seven targets); the owner decided on 2026-11-15
   that 0.52 ships the sixth archive (signed, verified on the winarm VM,
   with a sqlite prebuilt). Last verified 2026-11-15.
   Probe: `!body windows-arm64 https://github.com/xo-lang/xo/releases/latest/download/SHA256SUMS`
5. **`xo help build` names every target and library mode.** It still
   says windows/arm64 needs `--backend=go` (stale since 2026-11-09) and
   does not mention `--lib=shared|static`, linux/riscv64 or FreeBSD.
   Last verified 2026-11-15.
   Probe: `help windows/arm64[[:space:]]needs[[:space:]]--backend=go build`
   Probe: `!help --lib build`
6. **Native riscv64.** linux/riscv64 builds with the LLVM backend only
   (decision 0571); no native backend. Last verified 2026-11-15.
   Probe: `xo no[[:space:]]native[[:space:]]backend[[:space:]]builds[[:space:]]linux/riscv64 build --targets=linux/riscv64 -o out .`
7. **Editor lists for the tree-sitter grammar.** The grammar is
   published (Done, below); Helix's pull request is open (#16461,
   2026-11-09); nothing is submitted to nvim-treesitter, Zed's
   extensions or GitHub Linguist. Last verified 2026-11-15.
   Probe: `body "state":[[:space:]]*"open" https://api.github.com/repos/helix-editor/helix/pulls/16461`
   Probe: `body "total_count":[[:space:]]*0, https://api.github.com/search/issues?q=tree-sitter-xo+repo:nvim-treesitter/nvim-treesitter`
   Probe: `body "total_count":[[:space:]]*0, https://api.github.com/search/issues?q=tree-sitter-xo+repo:zed-industries/extensions`
   Probe: `body "total_count":[[:space:]]*0, https://api.github.com/search/issues?q=tree-sitter-xo+repo:github-linguist/linguist`
8. **A public process:** a front door for language proposals (the ADRs
   are the process, but outsiders can only send issues and patches,
   CONTRIBUTING.md "Code: as a proposal, not a merge"), a stated release
   cadence, and possibly a long-term-support line. Last verified
   2026-11-15.
   Probe: `repo as[[:space:]]a[[:space:]]proposal,[[:space:]]not[[:space:]]a[[:space:]]merge CONTRIBUTING.md`
   Probe: `!repo [Cc]adence CONTRIBUTING.md`
9. **The 1.0 compatibility freeze.** Decision 0567 states what 0.x
   promises and what 1.0 will; the freeze is the owner's, when the owner
   declares 1.0. Last verified 2026-11-15.
   Probe: `repo we[[:space:]]will[[:space:]]freeze[[:space:]]later knowledgebase/decisions/0567-compatibility-policy.md`

## Done

- **A stability promise.** Decision 0567 (Accepted, 2026-11-08), spec
  13, and the compatibility page. Last verified 2026-11-15.
  Probe: `repo ^Status:[[:space:]]Accepted knowledgebase/decisions/0567-compatibility-policy.md`
  Probe: `http 200 https://xolang.dev/docs/compatibility/`
- **A module proxy, checksum log and index** (decision 0568, Accepted
  2026-11-08): proxy.xolang.org, sum.xolang.org and index.xolang.org,
  with the client side in xo (`XO_PROXY`, `XO_SUMDB`; decisions 0124,
  0126). Last verified 2026-11-15.
  Probe: `body ^v0\.2\.1$ https://proxy.xolang.org/github.com/xo-lang/aws/@v/list`
  Probe: `body ^sum\.xolang\.org https://sum.xolang.org/latest`
  Probe: `body "Path":"github\.com/xo-lang/ https://index.xolang.org/index?limit=5`
  Probe: `body github\.com/xo-lang/aws[[:space:]]v0\.2\.1[[:space:]]h1: https://sum.xolang.org/lookup/github.com/xo-lang/aws@v0.2.1`
- **A security policy (how to report).** SECURITY.md, and each CHANGELOG
  security line names the affected versions. The database, the checker
  and the site page are Remaining items 1 and 2. Last verified
  2026-11-15.
  Probe: `body [Rr]eport https://raw.githubusercontent.com/xo-lang/xo/main/SECURITY.md`
- **Documentation for newcomers.** A 12-page tour whose examples run in
  the playground, the book, and browsable API docs: pkg.xolang.org
  (decision 0589) for std and published modules, and xolang.dev's std
  reference. Last verified 2026-11-15.
  Probe: `http 200 https://xolang.dev/docs/tour/`
  Probe: `http 200 https://xolang.dev/docs/stdlib/`
  Probe: `http 200 https://pkg.xolang.org/std/sqlite/`
  Probe: `http 200 https://pkg.xolang.org/github.com/xo-lang/aws/`
- **A published VS Code extension** ("Xo Language", `xo-lang.xo`, an
  `xo lsp` client that debugs through `xo debug --dap`; decisions 0113,
  0516) on the VS Code Marketplace and Open VSX, repository
  xo-lang/vscode. Last verified 2026-11-15 (0.1.3 on both).
  Probe: `http 200 https://marketplace.visualstudio.com/items?itemName=xo-lang.xo`
  Probe: `body "version":[[:space:]]*"0\.[0-9]+\.[0-9]+" https://open-vsx.org/api/xo-lang/xo`
- **A tree-sitter grammar,** xo-lang/tree-sitter-xo
  (infrastructure/tree-sitter-xo.md), published 2026-11-09 as npm
  `@xo-lang/tree-sitter-xo` and crates.io `tree-sitter-xo` (0.1.1). The
  editor lists are Remaining item 7. Last verified 2026-11-15.
  Probe: `body "latest":"0\.[0-9]+\.[0-9]+" https://registry.npmjs.org/@xo-lang%2ftree-sitter-xo`
  Probe: `body "max_version":"0\.[0-9]+\.[0-9]+" https://crates.io/api/v1/crates/tree-sitter-xo`
- **More platforms:** FreeBSD amd64 and arm64 (decision 0573) and
  linux/riscv64 through LLVM (decision 0571), both in 0.51.1; windows/arm64
  with the native backend by default (decision 0569, 2026-11-09). Native
  riscv64 and the windows/arm64 archive are Remaining items 6 and 4.
  Last verified 2026-11-15.
  Probe: `repo FreeBSD[[:space:]]\(decision[[:space:]]0573\) CHANGELOG.md`
  Probe: `repo riscv64 CHANGELOG.md`
  Probe: `repo windows/arm64[[:space:]]builds[[:space:]]with[[:space:]]the[[:space:]]native[[:space:]]arm64[[:space:]]backend[[:space:]]by[[:space:]]default CHANGELOG.md`
- **std docs on pkg.xolang.org at the release.** Refreshed to 0.51.1
  on 2026-11-15 (they were at v0.46.0); two commands per release
  (release CHECKLIST). Change the version in the probe at each release.
  Last verified 2026-11-15.
  Probe: `body std/json@v0\.51\.1 https://pkg.xolang.org/std/json/`
- **SQLite** (std/sqlite, decision 0534), beside Postgres. Last verified
  2026-11-15.
  Probe: `file std/sqlite.xo`
  Probe: `http 200 https://pkg.xolang.org/std/sqlite/`
- **Observability:** std/metrics (Prometheus exposition) and std/trace
  (decision 0325, spec 11.6), and OTLP export of both (std/otlp,
  decision 0539). Last verified 2026-11-15.
  Probe: `file std/otlp.xo`
  Probe: `http 200 https://pkg.xolang.org/std/otlp/`
- **Calling Xo from other languages:** `xo build --lib=shared|static`
  builds a C library (decision 0574), with a Windows DLL's import
  library. Last verified 2026-11-15.
  Probe: `repo xo[[:space:]]build[[:space:]]--lib=shared CHANGELOG.md`

## Related lessons from the 0.33.0 testers

Both testers' new bugs were in `use go` workflows (install, a fresh
clone, `mod tidy`, the newest Go), not the compiler. Tests of the
workflows a new user hits first (release smoke with `use go` and a
fresh-clone tidy, a newest-Go check) were milestone 0.34.0 work.
