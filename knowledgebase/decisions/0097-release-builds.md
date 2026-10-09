# 0097: Release builds are `xo build --release`; `--ci` does not imply it

Status: Accepted (2026-10-09, agent decision within the task "release
build mode", spec audit gap 3; the owner asked for `--release` and left
the `--ci` question to the implementer)

## Context

Core 8 says release builds may disable only the contract clauses marked
`requires(debug)` and `ensures(debug)`, and core 11.1 lets release builds
drop error frame parameters (`--error-args`, decision 0092). Until
2026-10-09 there was no release mode: the spec audit
(`bench/results/spec-audit-2026-10-09.md`, gap 3) marked both **[later]**,
and every build checked debug clauses. Decision 0086 used `xo build --ci`
as "the release build" for its workspace rule (XO1308) until a real mode
existed.

`--ci` today is a gate: it fails before building on an unformatted file or
a `dbg` left in code. Whether it should also switch the build mode was
open.

## Decision

- `xo build --release` is the release build. In it, and only in it, the
  clauses marked `(debug)` are compiled out, on the Go backend
  (`gen.Options.Release`) and on the native backends and `--hybrid`
  (`ssa.Options.Release`). `requires(debug) c else E` is compiled out too.
  Every other contract, invariant, and fault check stays.
- `--release` applies decision 0086's workspace rule: XO1308 while an
  xo.work workspace overrides a required module. `--ci` keeps reporting
  XO1308 as well, so existing pipelines behave as before.
- `--ci` does not imply `--release`. It stays a gate (format, `dbg`,
  workspace); a pipeline that ships the binary runs `xo build --ci
  --release`.
- `xo run` and `xo test` are always debug builds; `xo build` without
  `--release` is a debug build. `xo replay` rebuilds in the mode recorded
  in the build information (`"release": true`), so a replay of a release
  binary runs the same contracts.
- The build id and the Go build directory include the mode, so a debug and
  a release build of one program never share generated sources.

## Consequences

- `requires(debug)` and `ensures(debug)` have their specified meaning;
  core 8 and 10.1 lose their **[later]** markers for release builds.
- Golden pairs (`internal/gen/testdata/release/*/want.debug.txt`,
  `want.release.txt`) show the difference; `internal/build`
  `TestReleaseNative` checks the native IR; `cmd/xo` `TestBuildRelease`
  checks the flag and that `--ci` alone keeps debug contracts.
- `--error-args` (decision 0092) has a mode to belong to.
- A CI job that used `--ci` to mean "what we ship" still builds debug
  binaries until it adds `--release`. That is deliberate: turning
  contract checks off is a semantic choice that should be visible on the
  command line, not a side effect of a lint gate.

## Alternatives

- `--ci` implies `--release`: one flag for pipelines, but a lint gate
  would silently change which contracts run, and CI jobs that build to
  run tests would lose their debug checks. Rejected.
- Release as the default of `xo build` (debug for run and test only):
  matches Cargo's split in reverse; rejected because a binary built by an
  agent to try something should keep every check unless asked otherwise.
- `--profile=release|debug`: more general than needed while there are
  two modes; `--release` can become an alias if profiles ever appear.
