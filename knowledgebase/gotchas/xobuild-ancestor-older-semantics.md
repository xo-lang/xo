# xobuild built xo with an ancestor older than a language change

Status: Observed (2026-10-22, the hourly check at 43133450e)

Symptom: internal/gen TestGolden/mono_generics (Go backend) failed with a
compiler fault, "index 1526 out of range for List of length 1526 at
check/types.xo:135:19" (gen.Gen.map_last_use, is_prelude, nobj,
Checker.resolve), on every run of the cached xo of 43133450e's sources
(key ca3d6117074060a8). An xo built from the same sources by a newer xo
was correct; `xo check` and native builds were fine. A bisect over the
commits named b9092b2b6 (an AES-GCM change in std/native/llrt, which
changed the source key) though nothing in it was wrong.

Cause: xobuild builds a checkout's xo with "the newest earlier build"
(internal/xotool ancestorBuild), which took the cached build with the
newest file time among those whose commit is an ancestor of HEAD. A
worktree detached at d886e292d (70 commits older) had just rebuilt its
xo, so that build was chosen. d886e292d predates decision 0326 (a by
value receiver is read after the call's arguments), and gen's
map_last_use, written after 0326, calls `g.is_prelude(g.mono_type(..),
"Map")`, whose argument adds to g's types: the old xo read g for
is_prelude first, so the new type's index was past the copy's list. A
5,386 function difference from a correct build; the xo it made then
served everyone at that source key.

Fix: ancestorBuild takes the nearest ancestor (the fewest commits to
HEAD), not the most recently written one (TestAncestorBuild), and
map_last_use binds the type before the call. Code in compiler/ must not
depend on a language change newer than the builds xobuild may pick; a
change of evaluation order or semantics that the compiler's own sources
use needs a version bump and a seed (CLAUDE.md, seeds).

Observed again (2026-10-22, Windows CI run 39772751654): the nearest
ancestor is only as near as the cache allows. The xo-windows runner had
last built xo the evening before, so its nearest cached ancestor was at
least 214 commits behind main. xobuild built main's compiler with it in
one hop, and the xo it made refused nearly all of std/native with XO1201
("not available without a garbage collector (`--no-gc`) yet": names,
calls, assignments, tuple patterns), so every --race build failed. The
same build cross compiled from the Mac (a near ancestor cached) was fine.
Fix: when the builder is more than HopMax (50) commits behind HEAD, or
records no commit (`xo version --json`), Ensure builds compiler/xo again
with the xo it built, as the bootstrap's second stage
(internal/xotool SecondHop, TestSecondHop). A miscompiled first hop then
either builds a correct xo or fails, and the next seed is tried.
