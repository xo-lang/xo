# 0086: Modules, dependencies, and workspaces

Status: Accepted (2026-10-09, agreed with the user). Partially
implemented (2026-10-09): steps (1) MVS, git fetch, `xo.lock`, vendoring,
offline builds and (2) `xo.work` workspaces are built (`internal/mod`,
`xo get`, `xo mod tidy`, `xo vendor`, `--offline`, `--workspace=off`;
XO1301 to XO1308). Steps (3) effect ceilings, (4) `xo publish` semver
checks, and (5) per definition hashes are not; neither is `xo init`
gitignoring xo.work, an optional proxy, or a checksum log.

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
  in subdirectories of a repository and `/vN` major suffixes are not
  supported yet.
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
