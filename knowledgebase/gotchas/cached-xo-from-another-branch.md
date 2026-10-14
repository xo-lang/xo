# A cached xo from another branch fails to build this checkout's xo

Status: Observed (2026-10-14, CI run 38632539643).

Symptom: a CI log shows pages of `undefined reference to
xo_x_fault_raise_...` from `~/.cache/xo/build/<dir>/arm64/prog-linux.N.o`,
then "xotool: built in 56 s with the prebuilt seed". It reads like a
stale object leaking into the branch's link (it was first reported that
way), but the run's real failure was elsewhere.

Cause: internal/xotool tried "the newest earlier build" in the user
cache as the seed. On a CI runner that cache is shared by every branch,
so the newest build was another branch's xo (b06-entries). It emits
calls to runtime exports that branch added, which this checkout's
std/llrt lacks, so its build of xo failed to link and xotool fell back
to the seed.

Fix: buildLocked records the commit beside each build
(`<cache>/xo/checkout/<key>/commit`), and Seeds uses the newest earlier
build only when that commit is an ancestor of the checkout's HEAD
(ancestorBuild, TestAncestorBuild); builds with no record are skipped.
A seed that fails now logs one line (the first line and how many more);
the whole output is still in the error when no seed works.
