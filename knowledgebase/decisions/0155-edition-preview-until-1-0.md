# 0155: `edition preview` until 1.0

Status: Accepted (2026-10-11, by the owner). Implementation queued (the
rewrite of every xo.mod, the spec, diagnostics, and docs).

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
