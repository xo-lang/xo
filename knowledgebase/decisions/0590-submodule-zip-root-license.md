# 0590: a module in a subdirectory carries its repository's LICENSE

Status: Proposed (Azure 1, 2026-11-10; the owner decides). It changes the
content hash of some module versions, so it waits for the owner. The
change is on the branch `submodule-license`, gated, not pushed.

## Context

Decision 0589 (pkg.xolang.org) shows a module version's docs only when the
version carries a license ("no license, no docs", as pkg.go.dev). A module
in a subdirectory of its repository (core 4.2, decision 0086) is that
directory's tree at its tag, so it has no LICENSE unless the directory
holds its own. Most published Xo modules are such modules today:
github.com/xo-lang/aws's service modules, azure's 338 and gcp's 668 each
sit in a directory with no LICENSE, under a repository whose root has one.
pkg.xolang.org would show none of them.

Go solved this in cmd/go: modfetch (coderepo.go, `zip`) builds a
subdirectory module's zip from the directory and, when no file in it is
named exactly `LICENSE`, adds the repository root's `LICENSE` at the same
revision (at most codehost.MaxLICENSE, 16 MB). Only that exact name:
`LICENSE.md`, `COPYING` and `LICENCE` at the root are not copied, and a
module whose directory has any file named `LICENSE` keeps its own.
pkg.go.dev then finds the license in the zip like any other.

## Decision (proposed)

xo does what cmd/go does, at the one place a version's tree is made from
git (compiler/cli/modules fetch.xo `git_fetch`): after `git archive` of
`<tag>:<dir>`, when the module is in a subdirectory and its tree has no
`LICENSE`, the root's `LICENSE` at the tag (a blob, at most 16 MB) is
written into the tree as `LICENSE`. Everything downstream reads that tree:
the content hash (`h1:`, xo.lock and the checksum log's record), the proxy
zip (`zip_module`, `xo mod proxy fill`), vendor/, and `xo doc`. A root
module, a subdirectory module with its own `LICENSE`, and a repository
with no root `LICENSE` are unchanged.

## What changes, and for whom

- The content hash of every subdirectory module version that gains the
  file. A `h1:` line for such a version in an xo.lock written by xo 0.45.0
  or older stops matching a fresh fetch: `XO1304`.
  - Affected now: projects that require a subdirectory module of a
    repository with a root LICENSE. The known ones are the aws service
    modules (tagged v0.1.0); azure and gcp have no tags yet (the owner tags
    them with 0.46.0), so a lock for them can only come from a workspace,
    which has no hash line.
  - Not affected: the checksum log. sum.xolang.org has 0 records
    (`/healthz` on 2026-11-10), so no record holds an old hash. Shipping
    this before the log takes records is what keeps that true; after, the
    log's old records would disagree with every new fetch of those
    versions, which only a new log could fix.
  - The module cache: a version already in a cache keeps the tree it was
    fetched with (xo never refetches a cached version), so an old cache and
    an old lock still agree. A new lock with an old cache is `XO1304` too;
    `xo mod clean` refetches.
- Transition: when the tree differs from the xo.lock line only by its top
  level `LICENSE` (the hash without that file equals the line), `XO1304`'s
  note says so and names this decision: delete the line and run
  `xo mod tidy` to record the new hash. Any other mismatch keeps the usual
  note.

## Alternatives

- pkgsite only: the doc builder reads the repository root's LICENSE at
  the tag when a module's tree has none, and no hash changes. It keeps the
  toolchain as it is, but the license then lives only on the docs site:
  the module zip, the proxy and vendor/ still ship code without its license,
  which is what Go's rule exists to avoid, and the builder would need git
  access to the repository instead of the proxy's zip alone (decision 0589
  builds from the proxy). The hash change also gets harder with every day:
  today it touches only aws's service locks and no log record.
- Copy `LICENSE.*`, `COPYING` and `LICENCE` too: a wider rule than Go's.
  Go copies only `LICENSE`, and matching Go keeps xo's hashes and zips
  explainable by the same rule; a repository can always name its file
  `LICENSE`.
- Copy NOTICE as well (Apache-2.0 asks that NOTICE travel with the code):
  Go does not; left for a later decision if a module needs it.

## Tests

cmd/xo TestSubdirModuleLicense: the root LICENSE in a subdirectory module
and a two-level one (`deep/x`), a module's own LICENSE kept, nothing added
when the root has only `COPYING` and `LICENSE.md`, and `XO1304` with the
note for a lock holding the tree's hash without LICENSE (and without it for
any other mismatch).
