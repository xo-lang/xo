# 0086: Modules, dependencies, and workspaces

Status: Accepted (2026-10-09, agreed with the user). Partially
implemented (2026-10-09): steps (1) MVS, git fetch, `xo.lock`, vendoring,
offline builds and (2) `xo.work` workspaces are built (`internal/mod`,
`xo get`, `xo mod tidy`, `xo vendor`, `--offline`, `--workspace=off`;
XO1301 to XO1308), and so are (3) effect ceilings (`require ... uses`,
`use ... uses` in xo.work, XO1309, `xo get` API and effect diffs, `xo get
-u`) and (4) `xo publish [--check]` semver checks (`internal/release`,
XO1310, XO1311), `xo init` gitignoring xo.work, and `/vN` major version
paths. Step (5) per definition hashes is not built; neither is an
optional proxy or a checksum log. Modules in subdirectories of a
repository (tags `<dir>/vX.Y.Z`, as Go) are built too (2026-10-09, notes
below).

## Context

Spec 4.2 and 4.3 sketch `xo.mod` (`require`) and `xo.lock` (content
hashes) but nothing fetches, selects versions, or vendors. Other people
cannot build on Xo modules until that exists. The Argon2 work (decision
0059) showed offline builds matter: a dependency that needs the network
at build time broke the build with an empty module cache. Agents often
change a library and its caller together, so cross module work must be
cheap.

## Decision

Take Go's module system and add what Xo's effects make possible.

From Go:
- The module path is its location (`github.com/acme/billing`), fetched
  from git directly or through an optional proxy. No central registry.
- Minimal version selection: the build uses the lowest version that
  satisfies every `require`. No version ranges, no solver.
- `xo.lock` records content hashes; an optional public checksum log.
- No code runs at fetch or install: no build scripts, no hooks.
- `xo vendor` and fully offline builds once dependencies are cached.

Xo additions:
1. Effect ceilings per dependency in `xo.mod`
   (`require github.com/acme/billing v1.4.0 uses none`). An upgrade whose
   code needs an effect above the ceiling fails the build at the new call
   site.
2. Semver checked by the toolchain: `xo publish` compares the public API
   and effects (`xo query api`) with the previous version and rejects a
   minor or patch release that removes or changes an export or adds an
   effect to one. `xo get -u` prints API and effect changes.
3. Later (spec 4.3 [P3]): per definition hashes.

Workspaces: an `xo.work` file is the only way to point a dependency at
local code.

```
xo 2026
use ./api
use ./billing
use ../shared-lib
```

- Listed modules override `require` versions with the local copy; MVS
  for everything else.
- No `replace` directive in `xo.mod`.
- `xo.lock` is never rewritten with workspace overrides; workspace builds
  record local paths in build info only.
- Release builds fail while a workspace is active unless
  `--workspace=off` (or `XO_WORK=off`), naming the overridden modules.
- `xo check`, `test`, `edit`, `query`, `mcp`, and `serve` treat the
  workspace as one program (atomic cross module `xo edit`).
- Effect ceilings apply between workspace modules too.
- `xo.work` may be committed (monorepos); `xo init` gitignores it for
  single module repos.

Commands: `xo get <path>[@version]`, `xo mod tidy`, `xo vendor`,
`xo publish --check`. `use go` dependencies keep using Go's `go.mod`.

Order: (1) MVS, git fetch, `xo.lock`, vendoring, offline; (2) `xo.work`;
(3) effect ceilings; (4) enforced semver at publish; (5) per definition
hashes.

## Consequences

- Reproducible builds without a solver; upgrades only when asked.
- A compromised or careless dependency cannot gain network or file access
  silently.
- Version numbers become trustworthy for agents.
- No local path can leak into a published module.
- Hosting is git; a proxy and checksum log are optional services.

## Alternatives

- Central registry with version ranges and a solver (npm, Cargo): name
  squatting, nondeterministic resolution, install scripts. Rejected.
- Go's `replace` plus `go.work`: two overlapping mechanisms. Rejected for
  one (`xo.work`).
- Runtime permissions per process (Deno): coarser than per dependency
  effects checked at compile time.

## Implementation notes (2026-10-09)

- A module is its repository root; tags `vX.Y.Z` are versions. Modules
  in subdirectories of a repository: see the notes of 2026-10-09 below
  (this line said "not supported yet" until then). `/vN` major
  suffixes (N >= 2) are, Go's "major branch" way only: all major
  versions share one repository (the suffix is stripped for the git
  location), the tags of major N declare the `/vN` path in xo.mod, and a
  path without a suffix takes only v0 and v1 tags. The local name of an
  import skips the suffix.
- `xo mod tidy` drops a requirement nothing imports unless it raises a
  version the rest of the graph selects (Go keeps such lines as
  `// indirect`; Xo keeps them as plain requires).
- An import of a package from a module that is in the build list only
  because another module requires it is XO1306: each module requires
  what it imports.
- In a workspace, xo.lock verification uses the union of the workspace
  modules' locks; `xo get`, `xo mod tidy`, and `xo vendor` always ignore
  the workspace, so locks never record overrides.
- Release builds are `xo build --ci` until a `--release` mode exists
  (decision 0092 proposes one).

## Implementation notes, steps 3 and 4 (2026-10-09)

- A ceiling counts the effect labels a dependency's function declares
  (pub functions and methods always declare theirs), checked at every
  call (`checkCeiling` in `internal/check/ceiling.go`, from `callFunc`)
  and every use as a function value. Effect variables are the caller's
  effects passed through, so `apply[fx](f)` with an effectful callback is
  within `uses none`. Calls through an interface the dependency does not
  define are bounded by that interface's declared effects instead.
- Ceilings from xo.mod and from the workspace's `use` line both apply;
  the first one exceeded is reported.
- The API compared by `xo publish` and `xo get` is the syntax level
  surface of `xo query api` (`query.DeclSurface`) over every package of
  the module, so old versions need no dependencies fetched. A function's
  `uses` clause is compared as a set (adding `fs` where `fs.read` was is
  an added effect); any other signature difference is a change. Changes
  to struct fields or enum variants count as breaking (conservative:
  struct literals and exhaustive matches can break).
- Before v1.0.0 a minor release may break (Cargo's convention); a patch
  may not. Without `--version` the release is assumed compatible (next
  minor, or next patch before v1), so breaking changes fail by default.
- `xo publish` without `--check` also requires a clean git working tree,
  `mod.Load` without workspace, and `xo check` with `XO_WORK=off`, then
  prints `git tag vX.Y.Z && git push origin vX.Y.Z`; it runs neither.
- `xo init` gitignores `/xo.work` unless an xo.work already exists at or
  above the directory (a monorepo that commits its workspace).
- `xo serve` records, per module root, which xo.work a load would consult
  (`mod.NearestWork`), so a workspace created, removed, or edited above
  the directories makes the cached program stale.

## Implementation notes, modules in repository subdirectories (2026-10-09)

- As Go: the module `github.com/acme/tools/cli` may be the directory
  `cli/` of the repository `github.com/acme/tools`; its versions are the
  tags `cli/vX.Y.Z` and a version's tree is `cli/` at that tag (`git
  archive refs/tags/cli/v1.2.0:cli`). A `/vN` suffix stays out of the
  directory and the prefix: `github.com/acme/tools/cli/v2` is `cli/` with
  tags `cli/v2.x.y` (the major branch way; Go's major subdirectory layout
  is not supported).
- Finding the repository (`mod.Locate`): on github.com, gitlab.com, and
  bitbucket.org it is the first three path elements. Elsewhere `git
  ls-remote` is tried for each prefix of the path, longest first; there is
  no HTML meta discovery (Go's `?go-get=1`).
- A module's tree never holds a nested module: directories with their own
  xo.mod are removed after extraction and skipped by the content hash, so
  a vendored root module (which holds its vendored subdirectory modules)
  hashes as the fetched one. A root module's xo.lock hash changes only if
  its repository has nested xo.mod directories.
- `xo publish` in a subdirectory: the module path (without `/vN`) must
  end with the directory (XO1311 otherwise), the previous release is the
  highest `<dir>/vX.Y.Z` tag whose major the path admits, and the printed
  command is `git tag <dir>/vX.Y.Z && git push origin <dir>/vX.Y.Z`. The
  clean tree check covers the module's directory only (`git status
  --porcelain -- .`).
- Tests: `cmd/xo` `TestSubdirModules`, `internal/mod` `TestLocate`.
