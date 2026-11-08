# 0567: Compatibility policy: what 0.x promises today and what 1.0 will promise

Status: Accepted (owner, 2026-11-08: "you can write that policy now and then we will freeze later"). The 0.x rules below
are in force now; the 1.0 section is the promise as written in spec 13, frozen when the owner declares 1.0 (decision 0089).

## Context

Xo has `edition preview` (decision 0155), a rewriter for every break (decision 0089), stable diagnostic codes (spec 10) and a
changelog that marks breaking entries, but the rules were spread over those places and the spec. maturity-gaps.md item 1 lists
a written stability promise as the first thing a mature language has that Xo lacks. The owner decides when 1.0 is and
asked for the policy to be written now and frozen later.

## Decision

### What 0.x promises today

1. **`edition preview` is not frozen.** A minor release (0.N.0) may change the language, the prelude and std (spec 13,
   decisions 0089 and 0155). A patch release (0.N.P) is a fix with no spec change (0089). The toolchain minor matches the spec
   draft, and the spec's "Changes in 0.N" section lists every language change of a minor.
2. **Every break ships with a rewrite and a changelog line.** A change that can make code that built before stop building, or
   build with another meaning, lands in a minor release (never a patch release) with:
   - an `xo fix` rewrite in the same release (`xo fix --from=draft-0.N` for code written for an older draft, decision 0089),
     or, where no mechanical rewrite exists, a checker diagnostic that names the replacement;
   - a `CHANGELOG.md` entry marked **Breaking** that states the migration (the uuid entry of 0.34.0 is the model: what changed,
     each old form and its replacement).
3. **Deprecate first, remove in a later minor release.** A feature that is to go is deprecated in one minor release and
   removed in a later one, never in the same release. The deprecation is a diagnostic (a warning or note, with a fix where
   one exists) that names the replacement, and the changelog and the diagnostic table record it. The precedent is
   `uuid.Uuid`: its `u.0` and `uuid.Uuid(s)` forms were deprecated in 0.33.0 (vet's XO0912) and became type errors in 0.34.0
   (decision 0387, option (c)); the implied rule is at least one minor release between the announcement and the removal. A
   flag follows the same rule (`--hybrid` has warned on every use since 2026-10-22, spec 10.1). A deprecated form keeps
   its meaning while it is deprecated.
4. **Diagnostic codes never change meaning.** A code `XOnnnn` keeps the meaning it was first given; a new meaning gets a new
   code. A code that is no longer reported stays in the table of spec 10.6 marked retired (XO0912 is the example).
5. **Anything not listed in 1 to 4 may change in a minor release**, with rule 2 for what breaks code.

### What 1.0 will promise

This is spec 13, restated; the spec stays the source and this section follows it.

- **The first frozen edition.** At 1.0 the owner names the first frozen edition (whether editions are years or another
  scheme is decided then, decision 0155), and `xo fix` rewrites `edition preview` to it mechanically. The promise starts
  with that edition, not with `preview`.
- **No change of meaning within an edition.** Code that builds under an edition builds with the same meaning under every
  later toolchain. A new edition may change syntax and defaults, never the meaning of code in older editions, and the
  compiler supports every edition ever released. Modules on different editions link together.
- **Migration is mechanical.** `xo fix --edition=<name>` migrates a module and leaves it compiling; an edition is not
  released until its rewriter is complete.
- **std follows the same rule.** std APIs are never removed within an edition; a deprecated API ships a rewrite in `xo fix`.
- **Diagnostic codes** keep rule 4 above.

What 1.0 does not promise is not decided here: the backends' status at 1.0 (decision 0089 lists the Go backend as the
supported path and the native backends as preview), the toolchain's flags and output formats, and the length of the support
window for an old edition.

### Road to 1.0 (entry criteria, for the owner to edit)

1.0 stays the owner's call and has no date. These are the conditions this policy proposes before the owner declares it:

1. Every question of spec Appendix B is answered by a decision record.
2. Several outside projects have been in real use for some months.
3. A run of releases with no breaking change (the length of the run is the owner's).
4. One or two release candidates.
5. A tester round per release, as with 0.33.0.

The criteria of decision 0089 (the Go backend is the supported 1.0 path; the native backends ship as preview; native TLS is
not required) still stand.

### Proposed addendum (Xo Lang 1, 2026-11-08; the owner decides)

Not yet policy. Proposed line for "What 0.x promises today", rule 2:
**a new variant of a std enum that user code can match exhaustively (`TargetOs`, `TargetArch`, any `pub` std enum) is a
breaking change**: it ships only in a minor release, is marked **Breaking** in CHANGELOG.md with the migration (an arm for
the variant, or `_ =>`), and is listed in the spec's "Changes in 0.N". XO0301 names the missing variant, which counts as the
diagnostic naming the replacement, so no `xo fix` rewrite is required. Found by `TargetArch.Riscv64` (0571) and
`TargetOs.FreeBSD` (0573), which broke four goldens, two bench programs and three frozen blind solutions
(gotchas/std-enum-variant-breaks-exhaustive-matches.md). For 1.0 the owner also decides whether such enums are declared open
(a required `_ =>` arm outside std) so that adding a target stops being a break.

## Consequences

- Spec 13 links this decision, and maturity-gaps.md item 1 records that the policy is written.
- The website gets a compatibility page and a blog post (owner review first, then the lead publishes).
- Not implemented yet, a follow-up: a release step that reports a **Breaking** changelog entry with no rewrite or named
  replacement, and a removal whose deprecation shipped in the same release.
- At 1.0 this record is superseded by a decision that names the frozen edition and restates section "What 1.0 will promise"
  as the promise (decision 0155 and spec 13 already hold its text).

## Alternatives

- Wait until 1.0 to write anything: a user of 0.x cannot tell what a minor release may do to them; the uuid break shows the
  rule is already followed in practice.
- Promise no break without a deprecation: too strong before 1.0, while the language still moves and Appendix B has open
  questions.
- Copy Go's 1 compatibility promise now: it fits a frozen language; Xo's editions give the same promise from 1.0 on and leave
  room to change defaults before it.
