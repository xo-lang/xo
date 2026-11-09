# Off-main release seeds were skipped by xobuild

Status: Observed and fixed (2026-11-05, Xo Lang 2); observed again and fixed (2026-11-09, Xo Lang 2).

## Symptom

After seed 0.40.0-9bb97059 was uploaded (seeds/latest.txt named it), a clean
checkout of main with private caches built xo from seed 0.37.0-849e984, 609
commits behind, and rebuilt with that seed's output ("is 609 commits behind
HEAD ... building again"). On the Mac it still built (82 s); a Windows host
bootstrap, whose 0.37.0 seed lacks the PE import rows of 123e03243, could not
link windows programs, the gap the 0.40.0 seed was cut to close.

## Cause

FetchSeedFor (internal/xotool/seed.go) used a seed only when its commit was an
ancestor of HEAD, and otherwise the newest such seed. 0.39.0 (the respin) and
0.40.0 were tagged on a release commit made on the tested candidate while main
moved on: one commit of version strings, CHANGELOG and spec drafts whose parent
is on main, but which is never itself an ancestor of main. Both seeds were
skipped for 0.37.0's, the newest cut on main.

## Fix

seedPlaced: a seed's commit also counts when it is a release commit whose first
parent is an ancestor of HEAD, checked from the commit itself: its diff from
the parent touches only CHANGELOG.md, spec/core.md, spec/std.md, std/VERSION
and the two version constant files, and in compiler/ every changed line is
`pub const VERSION` or `pub const XO_VERSION`. A seed cut from a branch with
code changes is still refused (TestFetchSeedForAncestor).
commitsBehind counts such a seed's distance from its parent, so it is not
called "an unknown number of commits behind" and built a second time.

## Avoid

After a seed upload, check that a clean checkout of main fetches it (the
release plans' clean-checkout xobuild step reads the "with the prebuilt seed"
line for the seed's name, not only that the build passed).

## Again: a release commit on a fix made off main (2026-11-09)

On the owner's new Mac Studio (an empty xo cache), xobuild skipped seed
0.44.0-02382858 and fell back to 0.43.0, which cannot build main
(rt.bytes_skip32). The release commit 02382858's parent was e0f436271, a
cmd/xo testdata/clihelp fix made on the candidate and never on main, so
seedPlaced's one step to the parent did not reach main. seedBase
(internal/xotool/seed.go) now walks first parents, up to 20, over commits
off main that change only release files (as above) or only tests,
testdata and docs (`_test.go`, `_test.xo`, `testdata/`, `knowledgebase/`,
`.md`), to the first ancestor of HEAD, and commitsBehind counts from it;
0.44.0 now stands for b0ee4622c. TestFetchSeedForAncestor covers a release
commit on a testdata, test and docs fix (placed) and on a compiler fix
(refused).

Avoid: release-branch fixes land on main first, so the release commit's
parent is a main commit (toolchain.md "Versions and releases" step 1).
