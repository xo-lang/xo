# 0155: `edition preview` until 1.0

Status: Accepted (2026-10-11, by the owner), implemented (2026-10-11).

## Context

`xo.mod` declares an edition (spec 13), written as a year since the first
spec (2026-10-08, after Rust's editions); no decision recorded why. Every
module says `edition 2026`. Two problems: a year can read as a version
(`2026` as "v2026"), and if 1.0 ships after 2026 the first frozen edition
would look a year old on launch day. Before 1.0 nothing is frozen
(decision 0089), so the name can change at no compatibility cost.

## Decision

- Until 1.0, the only edition is `preview`: `edition preview` in xo.mod,
  `xo fix --edition=preview`. It states the 0.x status in every module.
- `edition 2026` is accepted as an alias of `preview` during the 0.x
  releases, so existing modules keep building; `xo fix` rewrites it, and
  `xo check` gives a fixable note.
- At 1.0 the owner names the first frozen edition, and `xo fix` rewrites
  `preview` to that name mechanically. Open for that decision: the owner
  noted that a year can look like a version number, so whether the frozen
  editions are years (Rust's model) or another scheme is decided then.

## Consequences

- One mechanical rewrite now: about 50 xo.mod files in the repo, spec
  4.2 and 13, `xo new` templates, diagnostics that print the edition,
  `check.CurrentEdition`, tests and goldens, the website's examples.
- The compatibility promise of spec 13 starts with the first frozen
  edition, not with `preview`.

## Alternatives

- Keep `2026` and rename at 1.0: pre-1.0 code and writing about Xo keep
  the old year.
- Numbered editions (`edition 1`): never dated, but read like toolchain
  versions (`edition 1` next to `xo 1.4`).

## Implementation (2026-10-11)

- `check.CurrentEdition` is the string `preview`; `check.Editions` lists
  edition names in order, `check.EditionAliases` maps `2026` to
  `preview`, `check.ResolveEdition` resolves both. `Module.Edition` is a
  string; `AtLeast` and `editionRule.Since` compare positions in
  `check.Editions`.
- `XO1315` (note, checker): `edition 2026` in the xo.mod of a checked
  module (not a dependency), once per xo.mod, with a fix that writes
  `preview`. `xo fix` loads xo.mod files into its overlay (those under its
  dirs and the one around each dir) so the fix applies; `xo check --fix`,
  the LSP, and MCP get it through the same fix.
- `xo fix --edition=` accepts `preview` and the alias `2026`.
- 48 tracked xo.mod files, the xo.work fixtures (`xo preview`), Go test
  fixtures, the bench prompts, spec 4.2, 10.1, 10.6, 13, and std.md were
  rewritten. Left on `2026`: `bench/paper/` (not to be edited), the
  vendored modules of `internal/check/testdata/golden/effect_ceiling_vendor`
  (their xo.lock hashes cover xo.mod; they now also show that a
  dependency on the alias builds without a note), and dated records
  (older ADRs, bench results and notes, CHANGELOG entries).
- The `xo <edition>` line of xo.work is still not validated (any word).
  Done 2026-10-11: `mod.ReadWork` resolves it with the resolver of
  xo.mod (the edition names moved to internal/mod): an unknown name is
  XO1301, `xo 2026` gets XO1315 (`` `xo 2026` is the old name of `xo
  preview` ``) and `xo fix` rewrites it; selfhost/check gives the same
  XO1301.
- The self hosted checker (selfhost/check) does not read `edition`
  lines, so nothing was ported; XO1315 is not one of the staged codes
  TestSelfhostChecker compares.

